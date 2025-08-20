package models

type UserWithChats struct {
	User  UserRow   `json:"user"`
	Chats []ChatRow `json:"chats"`
	Role  string    `json:"role"` // Role in each chat (if applicable)
}

type ChatWithMessages struct {
	Chat     ChatRow      `json:"chat"`
	Messages []MessageRow `json:"messages"`
	Users    []struct {
		User UserRow `json:"user"`
		Role string  `json:"role"`
	} `json:"users"`
}
