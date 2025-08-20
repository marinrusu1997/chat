# Chat Participant Constraints

---

## 👑 Ownership Rules
1. ✅ Only 1 owner per chat, exceptions for direct chat which can have 2 
2. ✅ Owner’s `user_id` must equal `chat.created_by`   
3. ✅ Owner cannot leave/rejoin/transfer
4. ✅ Owner cannot be banned, invited, kicked
5. ✅ Owner cannot be deleted unless entire chat deleted, except for direct chats
6. ✅ `user_id` = `chat.created_by` only if role = 'owner'
7. ✅ Auto-insert owner on chat creation
8. ✅ Thread chat owner inherited from parent

## 🧑‍🤝‍🧑 Role & Promotion Rules
9. ⚠️ Only admins+ can promote/demote **_(can't enforce, app should do this)_**
10. ❌ Admin/mod rejoin as member
11. ✅ Admin/mod must be invited/promoted, they can't join the chat as 'admin' or 'moderator'  
12. ✅ Guest cannot rejoin
13. ✅ Guest cannot be promoted directly to elevated roles

## 🤖 Bot Rules
14. ✅ Bot must have `color_theme` and `last_pinned_message_id` = NULL
15. ✅ Bot can never be owner, meaning it's a fixed role that can't be elevated
16. ✅ Bot role fixed at insert (cannot change)
17. ✅ Bots cannot invite or ban
18. ✅ Bot must have `invited_by` referencing human

## ⏱ Lifecycle Rules
19. ✅ `joined_at`, `invited_at` immutable  
20. ✅ `joined_at >= chat.created_at`
21. ✅ `left_at` and `rejoined_at` mutually exclusive
22. ❌ Rejoin must follow leave, `rejoined_at > left_at`, system clears left_at  
23. ❌ Leave clears rejoined_at

## 🚫 Ban Rules
24. ✅ On insert ban fields = NULL
25. ✅ If `banned_reason_note` not NULL  require `banned_reason_code`
26. ✅ (`banned_reason_code`, `ban_type`, `banned_by`) all NULL or all NOT NULL  
27. ✅ `banned_until` required for temporary bans, NULL for permanent
28. ✅ Clearing bans resets all fields
29. ✅ `banned_by` must be moderator+ with permission
30. ✅ Bans only updatable while active & requires the `banned_by`
31. ✅ Direct/self chats must have ban fields NULL

## 📩 Invitation Rules
32. ✅ `invited_by` must be moderator+ with permission and is required for secret chats
33. ✅ `invited_at >= chat.created_at`
34. ✅ `invited_at` and `invited_by` both NULL or both NOT NULL
35. ✅ If `invited_at` NULL  `invited_by` must be NULL
36. ✅ Immutable after insert, except reassignment to owner of `invited_by`
37. ✅ No self-invites (`invited_by != user_id`)
38. ✅ If non-owner deleted reassign `banned_by`/`invited_by`

## 📖 Message Tracking Rules
39. ✅ `last_read_message_id` and `last_read_at` both NULL or both set
40. ✅ When `last_read_message_id` is set to NULL, set `last_read_at` to NULL
41. ✅ When `last_read_message_id` is set to NOT NULL, set `last_read_at` to NOW()
42. ⚠️ `last_read_message_id` must belong to same chat **_(can't enforce, it belongs to ScyllaDB)_**

## 🏷 Chat Type Rules
43. ✅ Direct chat max 2 owners, different users
44. ✅ Self chat exactly 1 owner = `chat.created_by` 
45. ✅ Group chat exactly 1 owner (`chat.created_by`)
46. ✅ Thread chat inherits owner, immutable

## 🔒 Lock Rules
47. ✅ If chat locked, no new participants
48. ❌ If chat locked, block updates to `rejoined_at` field
49. ✅ If chat locked, block invites

## 🛠 Integrity Rules
50. ❌ Direct/self chat ban fields NULL, cannot leave/rejoin
