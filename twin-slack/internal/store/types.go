// Package store defines the Slack twin's state types and in-memory store.
package store

import "encoding/json"

// Channel represents a Slack channel (public, private, DM, or group DM).
type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsChannel  bool   `json:"is_channel"`
	IsGroup    bool   `json:"is_group"`
	IsIM       bool   `json:"is_im"`
	IsMPIM     bool   `json:"is_mpim"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived"`
	// IsMember is not stored state: handlers set it per request from Members
	// and the caller (see membershipView in the api package), as Slack computes
	// it per caller. Nothing writes it to the store, so a stored Channel reads
	// false here, and the admin state dump reports it that way.
	IsMember   bool     `json:"is_member"`
	Creator    string   `json:"creator"`
	Created    int64    `json:"created"`
	Topic      Topic    `json:"topic"`
	Purpose    Topic    `json:"purpose"`
	Members    []string `json:"members,omitempty"`
	NumMembers int      `json:"num_members"`
	// User is the other member of a direct message.
	User string `json:"user,omitempty"`
	// LastRead and IsOpen are the caller's read cursor and, for a direct or
	// multi-person message, whether it is open for them. Like IsMember they
	// are not stored: conversations.info fills them from the caller's
	// ReadState.
	LastRead string `json:"last_read,omitempty"`
	IsOpen   *bool  `json:"is_open,omitempty"`
}

// Topic holds a channel topic or purpose.
type Topic struct {
	Value   string `json:"value"`
	Creator string `json:"creator"`
	LastSet int64  `json:"last_set"`
}

// ReadState is one user's place in one conversation: the read cursor
// conversations.mark moves, and whether conversations.close closed a direct
// or multi-person message for them. It is keyed by ReadStateKey.
type ReadState struct {
	Channel  string `json:"channel"`
	User     string `json:"user"`
	LastRead string `json:"last_read,omitempty"`
	Closed   bool   `json:"closed,omitempty"`
}

// ReadStateKey is the key of a user's ReadState in a conversation.
func ReadStateKey(channel, user string) string { return channel + "/" + user }

// Message represents a Slack message.
type Message struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype,omitempty"`
	Channel  string `json:"channel,omitempty"`
	User     string `json:"user,omitempty"`
	BotID    string `json:"bot_id,omitempty"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"`
	Team     string `json:"team,omitempty"`
	Blocks   any    `json:"blocks,omitempty"`
	// Username and Icons are the per-message identity chat.postMessage sets
	// with username and icon_emoji.
	Username    string            `json:"username,omitempty"`
	Icons       map[string]string `json:"icons,omitempty"`
	Attachments []map[string]any  `json:"attachments,omitempty"`
	// Files are the files a file_share message shares.
	Files []File `json:"files,omitempty"`
	// Metadata is returned by conversations.history and replies only with
	// include_all_metadata.
	Metadata  map[string]any `json:"metadata,omitempty"`
	Reactions []Reaction     `json:"reactions,omitempty"`
	Edited    *MessageEdit   `json:"edited,omitempty"`
	// AppID and BotProfile identify the app and bot that posted a message
	// with a bot token, alongside BotID.
	AppID      string      `json:"app_id,omitempty"`
	BotProfile *BotProfile `json:"bot_profile,omitempty"`
	IsDeleted  bool        `json:"-"`
}

// Bot is a bot user's app identity, as bots.info returns it.
type Bot struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	Name    string `json:"name"`
	Updated int64  `json:"updated"`
	AppID   string `json:"app_id"`
	UserID  string `json:"user_id,omitempty"`
}

// BotProfile is the bot identity a bot's message carries.
type BotProfile struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	Name    string `json:"name"`
	Updated int64  `json:"updated"`
	AppID   string `json:"app_id"`
	UserID  string `json:"user_id,omitempty"`
	TeamID  string `json:"team_id"`
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
	Email           string `json:"email,omitempty"`
	DisplayName     string `json:"display_name"`
	DisplayNameNorm string `json:"display_name_normalized"`
	RealName        string `json:"real_name"`
	RealNameNorm    string `json:"real_name_normalized"`
	FirstName       string `json:"first_name,omitempty"`
	LastName        string `json:"last_name,omitempty"`
	Title           string `json:"title"`
	Phone           string `json:"phone"`
	// Skype is always empty: Slack no longer lets it be set.
	Skype            string `json:"skype"`
	Pronouns         string `json:"pronouns,omitempty"`
	StartDate        string `json:"start_date,omitempty"`
	StatusText       string `json:"status_text,omitempty"`
	StatusEmoji      string `json:"status_emoji,omitempty"`
	StatusExpiration int64  `json:"status_expiration,omitempty"`
	// Fields are the custom profile fields, keyed by field ID.
	Fields   map[string]ProfileField `json:"fields,omitempty"`
	Image24  string                  `json:"image_24,omitempty"`
	Image32  string                  `json:"image_32,omitempty"`
	Image48  string                  `json:"image_48,omitempty"`
	Image72  string                  `json:"image_72,omitempty"`
	Image192 string                  `json:"image_192,omitempty"`
	Image512 string                  `json:"image_512,omitempty"`
}

// ProfileField is the value of one custom profile field.
type ProfileField struct {
	Value string `json:"value"`
	Alt   string `json:"alt"`
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
	// Content holds the uploaded bytes. It is never rendered in an answer, and
	// is not kept in a state snapshot.
	Content []byte `json:"-"`
}

// ScheduledMessage holds a message scheduled for future delivery. The list
// item Slack shows is ID, Channel, PostAt, DateCreated and Text; the rest is
// what the message is posted with when post_at passes.
type ScheduledMessage struct {
	ID          string `json:"id"`
	Channel     string `json:"channel_id"`
	PostAt      int64  `json:"post_at"`
	DateCreated int64  `json:"date_created"`
	Text        string `json:"text"`

	// Token is the token that scheduled the message: chat.scheduledMessages.list
	// shows a token only the messages it scheduled.
	Token   string  `json:"token,omitempty"`
	Message Message `json:"message"`
	// Hold is set when the message will never post: Slack does not post a
	// scheduled message that carries metadata (chat.scheduleMessage docs).
	Hold bool `json:"hold,omitempty"`
}

// Team represents workspace info.
type Team struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Domain      string   `json:"domain"`
	EmailDomain string   `json:"email_domain"`
	Icon        TeamIcon `json:"icon"`
}

// TeamIcon is the workspace icon. A workspace that has not set an icon has
// ImageDefault true, and no image URLs are served.
type TeamIcon struct {
	ImageDefault bool `json:"image_default"`
}

// Bookmark represents a channel bookmark.
type Bookmark struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	Emoji     string `json:"emoji,omitempty"`
	Type      string `json:"type"` // "link"
	CreatedAt int64  `json:"date_created"`
	UpdatedAt int64  `json:"date_updated"`
}

// Usergroup represents a Slack user group (handle).
type Usergroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Handle      string `json:"handle"`
	Description string `json:"description"`
	// IsUsergroup is always true: Slack uses it to mark the object as a user
	// group. A disabled group is the one with a DateDelete.
	IsUsergroup bool           `json:"is_usergroup"`
	TeamID      string         `json:"team_id"`
	Users       []string       `json:"users,omitempty"`
	CreatedBy   string         `json:"created_by"`
	UpdatedBy   string         `json:"updated_by"`
	Prefs       UsergroupPrefs `json:"prefs"`
	DateCreate  int64          `json:"date_create"`
	DateUpdate  int64          `json:"date_update"`
	DateDelete  int64          `json:"date_delete"`
}

// UsergroupPrefs are a user group's default channels and groups.
type UsergroupPrefs struct {
	Channels []string `json:"channels"`
	Groups   []string `json:"groups"`
}

// View is a surface an app draws with Block Kit: a user's Home tab, or a
// modal. It is rendered as Slack's view payload.
type View struct {
	ID              string          `json:"id"`
	TeamID          string          `json:"team_id"`
	Type            string          `json:"type"`
	Title           json.RawMessage `json:"title,omitempty"`
	Close           json.RawMessage `json:"close"`
	Submit          json.RawMessage `json:"submit"`
	Blocks          json.RawMessage `json:"blocks"`
	PrivateMetadata string          `json:"private_metadata"`
	CallbackID      string          `json:"callback_id"`
	State           ViewState       `json:"state"`
	Hash            string          `json:"hash"`
	ClearOnClose    bool            `json:"clear_on_close"`
	NotifyOnClose   bool            `json:"notify_on_close"`
	SubmitDisabled  bool            `json:"submit_disabled,omitempty"`
	RootViewID      string          `json:"root_view_id"`
	PreviousViewID  *string         `json:"previous_view_id"`
	AppID           string          `json:"app_id"`
	ExternalID      string          `json:"external_id"`
	BotID           string          `json:"bot_id"`
}

// ViewState holds the values a user has entered in a view's input blocks.
type ViewState struct {
	Values map[string]any `json:"values"`
}

// ViewRecord is a stored view and the user it was published to. UserID is
// set for a Home tab view, which views.publish keeps one of per user.
type ViewRecord struct {
	UserID string `json:"user_id,omitempty"`
	View   View   `json:"view"`
}

// Token types, from the prefix Slack gives each credential.
const (
	TokenBot  = "bot"  // xoxb-
	TokenUser = "user" // xoxp-
	TokenApp  = "app"  // xapp-, app-level
)

// Token is a credential the app emulator recognises, and who it speaks for.
// A token the emulator has not been told about is still accepted when it is
// well formed (see ResolveToken), as a default principal of its type.
type Token struct {
	Token   string   `json:"token"`
	Type    string   `json:"type"`
	UserID  string   `json:"user_id,omitempty"`
	BotID   string   `json:"bot_id,omitempty"`
	TeamID  string   `json:"team_id,omitempty"`
	Scopes  []string `json:"scopes,omitempty"`
	Revoked bool     `json:"revoked,omitempty"`
	// OAuthCode is the authorization code the token was issued for, by
	// oauth.v2.access. A code is exchanged once.
	OAuthCode string `json:"oauth_code,omitempty"`
}

// Presence is a user's manual presence, set with users.setPresence. Slack
// keeps it for any user, including a bot user with no profile record here.
type Presence struct {
	User     string `json:"user"`
	Presence string `json:"presence"`
}

// MarshalJSON renders missing channel and group lists as [], as Slack does.
func (p UsergroupPrefs) MarshalJSON() ([]byte, error) {
	type prefs UsergroupPrefs
	out := prefs(p)
	if out.Channels == nil {
		out.Channels = []string{}
	}
	if out.Groups == nil {
		out.Groups = []string{}
	}
	return json.Marshal(out)
}
