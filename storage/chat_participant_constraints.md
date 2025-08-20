# Chat Participant Constraints

## 👑 Ownership Rules
1. Only **1 owner per chat**, except:
    - **direct chat** → exactly 2 owners, must be different users.
    - **self chat** → exactly 1 owner, must equal `created_by` from `chat` table.
2. Owner’s `user_id` must equal `chat.created_by`.
3. Owner cannot **leave**, **rejoin**, or **transfer ownership**.
4. Owner cannot be **banned, muted, invited, or deleted** while chat has other members.
5. Owner cannot ever be deleted unless the **entire chat is deleted**.
6. `user_id` may equal `chat.created_by` only if `role` = 'owner'.
7. On chat creation, insert the owner automatically as a participant.
8. In **thread chats**, owner is inherited from parent and immutable.

## 🧑‍🤝‍🧑 Role & Promotion Rules
9. Only **admins+** can promote/demote members.
10. When **admin/moderator leaves** and rejoins → they return as **member** with default member permissions.
11. Admins/Moderators must be **invited or promoted** (cannot self-join).
12. Guests cannot rejoin once they leave (must be re-invited).
13. Guests cannot be promoted directly to **owner/admin/moderator** (must become member first).

## 🤖 Bot Rules
14. Bots must have `color_theme` and `last_pinned_message_id` = NULL.
15. Bots can **never** be owners.
16. Bot role is fixed at insert → cannot be promoted/demoted.
17. Bots cannot invite, promote, or ban participants.
18. Bots must always have `invited_by` referencing a **human user**.

## ⏱ Lifecycle Rules
19. `joined_at` and `invited_at` are **immutable**.
20. `chat_participant.joined_at` ≥ `chat.created_at`.
21. `left_at` and `rejoined_at` are **mutually exclusive**:
    - both NULL, or
    - one set while other is NULL.
22. On rejoin (`rejoined_at`):
    - must follow a leave.
    - `rejoined_at > left_at`.
    - system clears `left_at`.
23. On leave (`left_at`):
    - system clears `rejoined_at`.

## 🚫 Ban Rules
24. On insert: all ban-related columns = NULL.
25. If `banned_reason_note` not NULL → `banned_reason_code` required.
26. (`banned_reason_code`, `ban_type`, `banned_by`) → either all NULL or all NOT NULL.
27. `banned_until` required if `ban_type`='temporary', must be NULL if `ban_type`='permanent'.
28. If ban is cleared → all ban fields reset to NULL.
29. `banned_by` must reference participant in same chat with role ≥ moderator and `CAN_BAN_MEMBERS`.
30. Bans can be updated only while **active**, only by the same banning moderator/owner, and only ban-related fields. Once cleared, no selective updates.
31. In **direct/self chats**, ban fields must always remain NULL.

## 📩 Invitation Rules
32. `invited_by` must reference participant in same chat with role ≥ moderator and `CAN_INVITE_USERS`.
33. `invited_at` ≥ `chat.created_at`.
34. `invited_at` and `invited_by` → both NULL or both NOT NULL.
35. If `invited_at` is NULL → `invited_by` must also be NULL (self-join).
36. Once set, `invited_at` and `invited_by` are **immutable**, except:
    - `invited_by` may be reassigned → **only to owner_id**.
37. No self-invites → `invited_by != user_id`.
38. If a non-owner participant is deleted → their `banned_by`/`invited_by` references are reassigned to the owner.

## 📖 Message Tracking Rules
39. `last_read_message_id` and `last_read_at` → both NULL or both set.
40. If `last_read_message_id` is NULL → force `last_read_at` = NULL.
41. Updating `last_read_message_id`:
    - If `last_read_at` missing → assign current timestamp.
    - New value must be ≥ old (monotonic → no “unread”).
42. `last_read_message_id` must reference a message **within the same chat**.

## 🏷 Chat Type Rules
43. **Direct chat**: max 2 owners, must be different users.
44. **Self chat**: exactly 1 owner, same as `chat.created_by`.
45. **Group chat**: exactly 1 owner (`chat.created_by`).
46. **Thread chat**: inherits owner from parent (immutable).

## 🔒 Lock Rules
47. If chat is locked → no new participants can be inserted.
48. If chat is locked → no updates to `rejoined_at`, `last_read_message_id`, `last_read_at`.
49. If chat is locked → no invites allowed.

## 🛠 Integrity Rules
50. If chat type is **direct** or **self** →
    - All ban fields must always be NULL.
    - Participants cannot leave/rejoin (`left_at` and `rejoined_at` always NULL).
