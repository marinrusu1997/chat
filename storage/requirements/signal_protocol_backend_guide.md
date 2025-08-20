# Backend Implementation Guide for a Signal Protocol-Based Chat App

This document serves as a reference guide for designing and implementing the backend services required for a secure chat application 
using the Signal Protocol. It covers the server's core responsibilities, security hardening techniques, and practical 
implementations for rate limiting.
* [Lecture 6. The Signal Protocol (Crypto 101: Real-World Deployments)](https://www.youtube.com/watch?v=JTLnCdQUOE4)
* [libsignal-protocol-go](https://github.com/RadicalApp/libsignal-protocol-go)

## 1. Core Server Responsibilities

While the Signal Protocol ensures end-to-end encryption, the server plays several critical roles beyond simply storing data. 
acts as an untrusted facilitator for key exchange and message delivery.

### 1.1. Public Key Storage (The Core Concept)

The server's primary cryptographic role is to act as a public key directory for users. For each registered user, the server must store:

* **Identity Key (`IK`):** A long-term public identity key for the user.
* **Signed Prekey (`SPK`):** A medium-term public key that is signed by the user's Identity Key. 
* The server must store both the key and its signature.
* **One-Time Prekeys (`OPK`):** A list of single-use public keys, uploaded in batches by the client.

### 1.2. The "Prekey Bundle"

When a user (Alice) wants to initiate a conversation with another user (Bob), she requests a **"prekey bundle"** from the server. 
The server is responsible for:

1.  Retrieving Bob's Identity Key (`IK_B`).
2.  Retrieving Bob's Signed Prekey (`SPK_B`) and its associated signature.
3.  Selecting **one** available One-Time Prekey (`OPK_B`) from Bob's list.
4.  Removing the chosen `OPK_B` from the list to prevent its reuse.
5.  Sending the complete bundle (`IK_B`, `SPK_B`, `signature`, `OPK_B`) to Alice.

### 1.3. Message Routing (The Mailbox)

The server acts as a store-and-forward message queue for encrypted messages (ciphertexts).

1.  Alice encrypts a message for Bob using his prekey bundle.
2.  She sends the resulting opaque ciphertext to the server, addressed to Bob.
3.  The server stores this ciphertext in a queue or "mailbox" for Bob.
4.  When Bob's client comes online, it authenticates and fetches any pending ciphertexts from the server.
5.  Bob's client decrypts the message locally.

### 1.4. User Registration and Authentication

The server must securely manage user identities to ensure keys are associated with the correct user.

* **Registration:** A secure process for creating new user accounts (e.g., via phone number verification).
* **Authentication:** Clients must authenticate with the server (e.g., using API keys, JWTs) before they can upload keys 
* or send/receive messages. This is crucial to prevent account takeover and impersonation.

### 1.5. Key Management and Housekeeping

* **OPK Depletion:** The server must handle cases where a user has run out of One-Time Prekeys. The protocol can fall back 
* to the Signed Prekey, but this has reduced forward secrecy guarantees for the initial message.
* **Client Notifications:** The server should notify clients when their `OPK` count is low, prompting them to upload a new batch.

### 1.6. Push Notifications

To avoid constant polling and save battery life, the server must integrate with platform-specific 
push notification services (Apple's APNs, Google's FCM).

1.  When a message arrives for an offline user, the server stores it.
2.  The server sends a "silent" push notification to the user's device.
3.  **Crucially, this notification must not contain any sensitive information.** It is merely a "wake-up" signal.
4.  The device's app wakes up, connects to the server, and fetches the actual message.

---

## 2. Hardening the Server: Preventing Prekey Bundle Exhaustion

A key security concern is a malicious client rapidly requesting a user's prekey bundles to "burn through" their One-Time Prekeys. 
This is a denial-of-service attack that degrades the protocol's forward secrecy. A multi-layered defense is required.

### 2.1. Strong Authentication
The endpoint serving prekey bundles **must** be authenticated. Only valid, registered users should be able to make requests. 
This makes attacks traceable to a specific malicious account.

### 2.2. Per-User Rate Limiting (The Core Defense)
This is the most direct defense. Limit how many times a single authenticated user can request prekey bundles within 
a given time frame (e.g., "User Alice can only request 200 bundles per hour").

### 2.3. Proof-of-Work
For sensitive actions, require the client to solve a small, computationally expensive puzzle. This makes each request 
expensive for the attacker, rendering high-volume attacks economically infeasible.

### 2.4. IP-Based Blocking & Circuit Breakers
For large-scale attacks from a limited set of IPs, use a Web Application Firewall (WAF) or tools like `fail2ban` to 
temporarily block malicious IP addresses at the network edge.

### 2.5. Monitoring and Anomaly Detection
* Monitor the rate of prekey bundle requests per user.
* Set up alerts for unusual spikes in requests for a specific user.
* This allows for manual or automated intervention to stop an attack in progress.

---

## 3. Practical Implementation of Rate-Limiting Algorithms with Redis

Redis is an ideal tool for implementing distributed rate limiters due to its speed and atomic operations. 
Below are two common, effective algorithms.

### 3.1. Token Bucket Algorithm

Controls the average rate while allowing for controlled bursts of requests.

* **Analogy:** A bucket with a fixed capacity is refilled with "tokens" at a constant rate. Each request consumes one token. 
* If the bucket is empty, the request is denied.
* **Data Structure in Redis:** A `HASH` per user.
    * **Key:** `user:{user_id}:token_bucket`
    * **Fields:** `tokens` (current count), `last_refill_ts` (timestamp).
* **Python Implementation:**

```python
import redis
import time

# Connect to Redis
r = redis.Redis(decode_responses=True)

def allow_request_token_bucket(user_id: str, capacity: int, refill_rate_per_sec: float) -> bool:
    """
    Implements the Token Bucket algorithm.

    :param user_id: The ID of the user making the request.
    :param capacity: The maximum number of tokens (burst capacity).
    :param refill_rate_per_sec: How many tokens are added per second.
    :return: True if the request is allowed, False otherwise.
    """
    bucket_key = f"user:{user_id}:token_bucket"
    current_ts = time.time()

    # Use a pipeline for an atomic transaction
    pipe = r.pipeline()
    pipe.hgetall(bucket_key)
    results = pipe.execute()

    bucket = results[0]

    if not bucket:
        # First-time request, initialize the bucket
        tokens = float(capacity - 1)
        pipe.hset(bucket_key, mapping={"tokens": tokens, "last_refill_ts": current_ts})
        pipe.execute()
        return True

    last_tokens = float(bucket.get("tokens", capacity))
    last_refill_ts = float(bucket.get("last_refill_ts", current_ts))

    time_elapsed = current_ts - last_refill_ts
    new_tokens = time_elapsed * refill_rate_per_sec
    
    # Add new tokens, but don't exceed the capacity
    current_tokens = min(float(capacity), last_tokens + new_tokens)

    if current_tokens >= 1:
        # Consume one token and update the bucket
        pipe.hset(bucket_key, mapping={
            "tokens": current_tokens - 1,
            "last_refill_ts": current_ts
        })
        pipe.execute()
        return True
    else:
        # Not enough tokens, reject the request
        pipe.hset(bucket_key, "last_refill_ts", current_ts)
        pipe.execute()
        return False
```

### 3.2. Sliding Window Counter Algorithm

Enforces a strict maximum number of requests over a rolling time period, preventing boundary condition issues of fixed windows.

* **Concept:** Store a timestamp for each request. Count how many timestamps fall within the current time window (e.g., the last 60 seconds).
* **Data Structure in Redis:** A `ZSET` (Sorted Set) per user.
    * **Key:** `user:{user_id}:sliding_window`
    * **Members:** A unique value for each request (e.g., `timestamp:nonce`).
    * **Scores:** The Unix timestamp of the request.
* **Python Implementation:**

```python
import redis
import time
import uuid

# Connect to Redis
r = redis.Redis(decode_responses=True)

def allow_request_sliding_window(user_id: str, limit: int, window_in_seconds: int) -> bool:
    """
    Implements the Sliding Window Counter algorithm.

    :param user_id: The ID of the user making the request.
    :param limit: The maximum number of requests in the window.
    :param window_in_seconds: The duration of the window in seconds.
    :return: True if the request is allowed, False otherwise.
    """
    key = f"user:{user_id}:sliding_window"
    current_ts = time.time()
    window_start = current_ts - window_in_seconds

    # Use a pipeline for an atomic transaction
    pipe = r.pipeline()

    # 1. Remove all outdated entries from the sorted set.
    pipe.zremrangebyscore(key, 0, window_start)

    # 2. Get the current count of requests in the window.
    pipe.zcard(key)
    
    # Execute both commands and get the results
    results = pipe.execute()
    current_count = results[1] # The result of zcard

    if current_count < limit:
        # 3. Add the new request timestamp to the set.
        pipe.zadd(key, {f"{current_ts}:{uuid.uuid4()}": current_ts})
        pipe.execute()
        return True
    else:
        # Limit reached, reject.
        return False
```

### 3.3. Algorithm Comparison and Recommendation

| Feature            | Token Bucket                                       | Sliding Window Counter                          |
|--------------------|----------------------------------------------------|-------------------------------------------------|
| **Primary Goal**   | Control average rate, allow controlled bursts.     | Enforce a strict limit over a rolling period.   |
| **Complexity**     | Slightly more complex logic (calculating refills). | Simpler logic with Redis `ZSET`s.               |
| **Memory Usage**   | Constant per user (2 fields in a hash).            | Proportional to request rate within the window. |
| **Recommendation** | Good for flexible plans (e.g., API usage tiers).   | **Excellent for security-critical limits.**     |

For the specific use case of preventing `OPK` burnout, the **Sliding Window Counter** is an excellent choice. 
It is simple to implement correctly with Redis and directly enforces the rule "no user may request more than X bundles in Y minutes," 
which is precisely the security guarantee you need.