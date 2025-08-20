# Chat App Permissions Reference (64-bit bitmask)

## 1️⃣ Full Permissions Table (Bitmask 0–63) with Categories

| Bit | Permission                  | Category                                                | Role (min)       | Description                                       |
|-----|-----------------------------|---------------------------------------------------------|------------------|---------------------------------------------------|
| 0   | `CAN_SEND_MESSAGES`         | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can send text messages                            |
| 1   | `CAN_SEND_MEDIA`            | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can send media (images, videos, audio)            |
| 2   | `CAN_SEND_VOICE`            | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can send voice messages                           |
| 3   | `CAN_SEND_VIDEO`            | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can send video messages                           |
| 4   | `CAN_SEND_FILES`            | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can send files                                    |
| 5   | `CAN_MENTION_ALL`           | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Can mention `@all` or `@everyone`                 |
| 6   | `CAN_REPLY_TO_MESSAGES`     | <span style="color:#808CD1;">_Interaction_</span>       | **member**       | Can reply to messages                             |
| 7   | `CAN_REACT_TO_MESSAGES`     | <span style="color:#808CD1;">_Interaction_</span>       | **member**       | Can react to messages (emoji reactions)           |
| 8   | `CAN_FORWARD_MESSAGES`      | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can forward messages                              |
| 9   | `CAN_FORWARD_OWN_MESSAGES`  | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can forward own messages                          |
| 10  | `CAN_EDIT_OWN_MESSAGES`     | <span style="color:#80D1CD;">_Editing_</span>           | **member**       | Can edit own messages                             |
| 11  | `CAN_DELETE_OWN_MESSAGES`   | <span style="color:#80D1CD;">_Editing_</span>           | **member**       | Can delete own messages                           |
| 12  | `CAN_DELETE_OWN_MEDIA`      | <span style="color:#BDD180;">_Media_</span>             | **member**       | Can delete own media                              |
| 13  | `CAN_POST_POLLS`            | <span style="color:#80D19E;">_Messaging_</span>         | **member**       | Can create, send and delete polls                 |
| 14  | `CAN_MANAGE_EMOJIS`         | <span style="color:#D18080;">_Chat Management_</span>   | **member**       | Can add/remove custom emojis                      |
| 15  | `CAN_MANAGE_REACTIONS`      | <span style="color:#D18080;">_Chat Management_</span>   | **member**       | Can add, edit and remove reactions (extra emojis) |
| 16  | `CAN_START_CALL`            | <span style="color:#BDD180;">_Media_</span>             | **member**       | Can initiate voice/video calls                    |
| 17  | `CAN_VIEW_ARCHIVED_THREADS` | <span style="color:#809AD1;">_Threads_</span>           | **member**       | Can see archived threads                          |
| 18  | `CAN_INVITE_MEMBERS`        | <span style="color:#A080D1;">_Membership_</span>        | **member**       | Can invite users via links                        |
| 19  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 20  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 21  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 22  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 23  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 24  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 25  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 26  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 27  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 28  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **member**       | Reserved for future use                           |
| 29  | `CAN_EDIT_CHAT_THEME`       | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can edit chat theme (colors, icons)               |
| 30  | `CAN_PIN_GLOBAL_MESSAGES`   | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can pin messages for all members                  |
| 31  | `CAN_PIN_GLOBAL_MEDIA`      | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can pin media globally                            |
| 32  | `CAN_MANAGE_TAGS`           | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can manage chat tags                              |
| 33  | `CAN_MANAGE_WEBHOOKS`       | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can manage chat webhooks                          |
| 34  | `CAN_MODERATE_THREADS`      | <span style="color:#809AD1;">_Threads_</span>           | **moderator**    | Can moderate threads                              |
| 35  | `CAN_MANAGE_INTEGRATIONS`   | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can manage bot integrations                       |
| 36  | `CAN_CREATE_ANNOUNCEMENTS`  | <span style="color:#D18080;">_Chat Management_</span>   | **moderator**    | Can create announcements                          |
| 37  | `CAN_MANAGE_FILES`          | <span style="color:#BDD180;">_Media_</span>             | **moderator**    | Can manage shared and other's files               |
| 38  | `CAN_DELETE_MESSAGES`       | <span style="color:#D1A280;">_Moderation_</span>        | **moderator**    | Can delete others' messages                       |
| 39  | `CAN_ADD_MEMBERS`           | <span style="color:#A080D1;">_Membership_</span>        | **moderator**    | Can add new members                               |
| 40  | `CAN_REMOVE_MEMBERS`        | <span style="color:#A080D1;">_Membership_</span>        | **moderator**    | Can remove members                                |
| 41  | `CAN_MANAGE_INVITES`        | <span style="color:#A080D1;">_Membership_</span>        | **moderator**    | Can revoke invites                                |
| 42  | `CAN_BAN_MEMBERS`           | <span style="color:#D1A280;">_Moderation_</span>        | **moderator**    | Can ban/kick members                              |
| 43  | `CAN_MUTE_MEMBERS`          | <span style="color:#D1A280;">_Moderation_</span>        | **moderator**    | Can mute members                                  |
| 44  | `CAN_VIEW_ANALYTICS`        | <span style="color:#C5D180;">_Analytics_</span>         | **moderator**    | Can view chat analytics                           |
| 45  | `CAN_VIEW_MODERATION_LOG`   | <span style="color:#C5D180;">_Analytics_</span>         | **moderator**    | Can view moderation log                           |
| 46  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **moderator**    | Reserved for future use                           |
| 47  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **moderator**    | Reserved for future use                           |
| 48  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **moderator**    | Reserved for future use                           |
| 49  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **moderator**    | Reserved for future use                           |
| 50  | `CAN_ASSIGN_ROLES`          | <span style="color:#A080D1;">_Membership_</span>        | **admin**        | Can assign roles                                  |
| 51  | `CAN_SET_PERMISSIONS`       | <span style="color:#A080D1;">_Membership_</span>        | **admin**        | Can modify other members’ permissions             |
| 52  | `CAN_EDIT_CHAT`             | <span style="color:#D18080;">_Chat Management_</span>   | **admin**        | Can change chat info (name, description, theme)   |
| 53  | `CAN_ARCHIVE_CHAT`          | <span style="color:#D18080;">_Chat Management_</span>   | **admin**        | Can archive chat                                  |
| 54  | `CAN_LOCK_CHAT`             | <span style="color:#D18080;">_Chat Management_</span>   | **admin**        | Can lock chat (prevent posting)                   |
| 55  | `CAN_VIEW_STATISTICS`       | <span style="color:#C5D180;">_Analytics_</span>         | **admin**        | Can view chat statistics                          |
| 56  | `CAN_USE_ADVANCED_FEATURES` | <span style="color:#CF9655;">_Advanced Features_</span> | **admin**        | Can access beta/advanced features                 |
| 57  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **admin**        | Reserved for future use                           |
| 58  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **admin**        | Reserved for future use                           |
| 59  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **admin**        | Reserved for future use                           |
| 60  | `CAN_DELETE_CHAT`           | <span style="color:#D18080;">_Chat Management_</span>   | **owner**        | Can delete the chat entirely                      |
| 61  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **owner**        | Reserved for future use                           |
| 62  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **owner**        | Reserved for future use                           |
| 63  | `RESERVED`                  | <span style="color:#C74040;">_Reserved_</span>          | **owner**        | Reserved for future use                           |

---

## 2️⃣ Default Permissions by Role (64-bit)

| Role           | Default permissions_bitmask | Permissions Set Description                               |
|----------------|-----------------------------|-----------------------------------------------------------|
| **owner**      | `0xffffffffffffffff`        | All 64 bits set (full permissions)                        |
| **admin**      | `0xfffffffffffffff0`        | All except bits 60-63 (chat management)                   |
| **moderator**  | `0xffffffffffffc000`        | All except bits 50-63 (chat moderation)                   |
| **member**     | `0xfffffff800000000`        | All except bits 29-63 (basic messaging & common features) |
| **guest**      | `0x0000000000000000`        | No permissions (read-only)                                |
| **bot**        | `0x0000000000000000`        | Flexible, assigned by application                         |

---

### Notes
- Future permissions can be set where `RESERVED` permissions are for each role to avoid breaking existing mappings.
