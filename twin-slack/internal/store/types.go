// Package store defines the Slack twin's state types and in-memory store.
package store

// Channel represents a Slack channel (public, private, DM, or group DM).
type Channel struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	IsChannel  bool     `json:"is_channel"`
	IsGroup    bool     `json:"is_group"`
	IsIM       bool     `json:"is_im"`
	IsMPIM     bool     `json:"is_mpim"`
	IsPrivate  bool     `json:"is_private"`
	IsArchived bool     `json:"is_archived"`
	IsMember   bool     `json:"is_member"`
	Creator    string   `json:"creator"`
	Created    int64    `json:"created"`
	Topic      Topic    `json:"topic"`
	Purpose    Topic    `json:"purpose"`
	Members    []string `json:"members,omitempty"`
	NumMembers int      `json:"num_members"`
}

// Topic holds a channel topic or purpose.
type Topic struct {
	Value   string `json:"value"`
	Creator string `json:"creator"`
	LastSet int64  `json:"last_set"`
}

// Message represents a Slack message.
type Message struct {
	Type      string       `json:"type"`
	Subtype   string       `json:"subtype,omitempty"`
	Channel   string       `json:"channel,omitempty"`
	User      string       `json:"user,omitempty"`
	BotID     string       `json:"bot_id,omitempty"`
	Text      string       `json:"text"`
	TS        string       `json:"ts"`
	ThreadTS  string       `json:"thread_ts,omitempty"`
	Team      string       `json:"team,omitempty"`
	Blocks    any          `json:"blocks,omitempty"`
	Reactions []Reaction   `json:"reactions,omitempty"`
	Edited    *MessageEdit `json:"edited,omitempty"`
	IsDeleted bool         `json:"-"`
}

// MessageEdit records who edited a message and when.
type MessageEdit struct {
	User string `json:"user"`
	TS   string `json:"ts"`
}

// Reaction represents an emoji reaction on a message.
type Reaction struct {
	Name  string   `json:"name"`
	Users []string `json:"users"`
	Count int      `json:"count"`
}

// User represents a Slack workspace user.
type User struct {
	ID       string      `json:"id"`
	TeamID   string      `json:"team_id"`
	Name     string      `json:"name"`
	RealName string      `json:"real_name"`
	Deleted  bool        `json:"deleted"`
	IsBot    bool        `json:"is_bot"`
	IsAdmin  bool        `json:"is_admin"`
	IsOwner  bool        `json:"is_owner"`
	Profile  UserProfile `json:"profile"`
	Updated  int64       `json:"updated"`
	TZ       string      `json:"tz,omitempty"`
	TZLabel  string      `json:"tz_label,omitempty"`
	TZOffset int         `json:"tz_offset,omitempty"`
	Presence string      `json:"presence,omitempty"`
}

// UserProfile holds profile fields for a Slack user.
type UserProfile struct {
	Email               string `json:"email,omitempty"`
	DisplayName         string `json:"display_name"`
	DisplayNameNorm     string `json:"display_name_normalized"`
	RealName            string `json:"real_name"`
	RealNameNorm        string `json:"real_name_normalized"`
	StatusText          string `json:"status_text,omitempty"`
	StatusEmoji         string `json:"status_emoji,omitempty"`
	Image24             string `json:"image_24,omitempty"`
	Image32             string `json:"image_32,omitempty"`
	Image48             string `json:"image_48,omitempty"`
	Image72             string `json:"image_72,omitempty"`
	Image192            string `json:"image_192,omitempty"`
	Image512            string `json:"image_512,omitempty"`
}

// Pin represents a pinned item in a channel.
type Pin struct {
	Type    string  `json:"type"`
	Channel string  `json:"channel"`
	Message Message `json:"message"`
	Created int64   `json:"created"`
	Creator string  `json:"created_by"`
}

// File represents a Slack file upload.
type File struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Title              string   `json:"title"`
	MimeType           string   `json:"mimetype"`
	FileType           string   `json:"filetype"`
	Size               int      `json:"size"`
	User               string   `json:"user"`
	Created            int64    `json:"created"`
	Timestamp          int64    `json:"timestamp"`
	Channels           []string `json:"channels,omitempty"`
	URLPrivate         string   `json:"url_private,omitempty"`
	URLPrivateDownload string   `json:"url_private_download,omitempty"`
	Permalink          string   `json:"permalink,omitempty"`
	IsPublic           bool     `json:"is_public"`
}

// ScheduledMessage holds a message scheduled for future delivery.
type ScheduledMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel_id"`
	PostAt    int64  `json:"post_at"`
	DateCreated int64  `json:"date_created"`
	Text      string `json:"text"`
}

// Team represents workspace info.
type Team struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}
