# Chat Application Functional Query List

## Core Messaging & Chat Lifecycle
1.  **Send a New Message**: Write the content, sender ID, chat ID, and creation timestamp of a new message.
2.  **Fetch a Page of Recent Messages**: Retrieve the latest `N` messages for a given chat.
3.  **Fetch Older Messages (Infinite Scroll)**: Retrieve `N` messages in a chat that occurred *before* a specific message timestamp/ID.
4.  **Fetch a Single Message by ID**: Retrieve the full details of a specific message.
5.  **Edit a Message**: Update the content and set the `edited` status for a specific message.
6.  **Delete a Message**: Remove a specific message from a chat.

## User Inbox & State Management
7.  **Fetch User's Inbox**: Retrieve a user's list of chats, sorted by the timestamp of the last message in each chat (newest first).
8.  **Update Inbox on New Message**: For all participants of a chat, update their inbox view with the new last message preview and timestamp.
9.  **Mark a Chat as Read**: Update the "last read message ID" for a specific user in a specific chat.
10. **Fetch All Unread States**: For a given user, retrieve their "last read message ID" for all of their chats.
11. **Leave a Chat**: Remove a specific chat from a user's inbox list and read state.

## In-Chat Features & Experience
12. **Add a Reaction**: Add a specific emoji reaction from a user to a message.
13. **Remove a Reaction**: Remove a specific emoji reaction from a user on a message.
14. **Fetch All Reactions for a Message**: Retrieve all reactions for one or more messages.
15. **Pin a Message**: Add a message to the list of pinned messages for a chat.
16. **Unpin a Message**: Remove a message from the list of pinned messages.
17. **Fetch Pinned Messages**: Retrieve the list of all pinned messages for a given chat.
18. **View All Media/Files in a Chat**: Fetch a paginated list of all attachments (images, videos, files) shared in a chat, sorted by newest first.
19. **Look up Attachment Details**: Retrieve the metadata for a single attachment by its ID.

## User, Social & Discovery Features
20. **Fetch User Profile**: Retrieve a user's public profile information (name, avatar, status) by their user ID.
21. **Fetch User Mentions**: For a given user, retrieve a list of all messages where they were mentioned, sorted by newest first.
22. **Fetch Chat Members**: Retrieve a paginated list of all members in a given chat.
23. **Global Message Search (Advanced)**: Find all messages containing a specific keyword across all chats a user is in.
24. **In-Chat Message Search (Advanced)**: Find all messages containing a specific keyword within a specific chat.