// Copyright (C) 2021 - 2025 PurpleSec Team
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package forwarder

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	addFailed        uint8 = 0
	addAlreadyExists       = iota
	addIsNotImage
	addSuccess
)

var emptyJpeg = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x60, 0x00, 0x60, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43, 0x00, 0x03, 0x02, 0x02, 0x02, 0x02, 0x02, 0x03, 0x02, 0x02, 0x02, 0x03, 0x03, 0x03, 0x03, 0x04,
	0x06, 0x04, 0x04, 0x04, 0x04, 0x04, 0x08, 0x06, 0x06, 0x05, 0x06, 0x09, 0x08, 0x0A, 0x0A, 0x09, 0x08, 0x09, 0x09, 0x0A, 0x0C, 0x0F, 0x0C, 0x0A, 0x0B, 0x0E, 0x0B, 0x09, 0x09, 0x0D, 0x11, 0x0D, 0x0E, 0x0F, 0x10, 0x10, 0x11, 0x10, 0x0A, 0x0C,
	0x12, 0x13, 0x12, 0x10, 0x13, 0x0F, 0x10, 0x10, 0x10, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x09, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0x54, 0xDF, 0xFF, 0xD9,
}

type photos []telegram.PhotoSize

func (p photos) Len() int {
	return len(p)
}
func (p photos) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
func (p photos) Less(i, j int) bool {
	return p[i].FileSize > p[j].FileSize
}
func (c *container) isAuthorized(u int64) bool {
	for i := range c.users {
		if u == c.users[i] {
			return true
		}
	}
	return false
}
func splitTags(d string) []telegram.MessageEntity {
	var (
		v = []byte(d)
		e []telegram.MessageEntity
		t = -1
	)
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '#':
			if t == -1 {
				t = i
				break
			}
			if n := i - t; n > 0 {
				e = append(e, telegram.MessageEntity{Offset: t, Length: n, Type: "hashtag"})
			}
			t = i
		// Break Chracters
		case ' ', '/', '\\', '[', ']', '~', '!', '@', '$', '%', '^', '&', '*', '(', ')', '=', '|', ':', ';', ',', '"', '\'':
			if t == -1 {
				break
			}
			if n := i - t; n > 0 {
				e = append(e, telegram.MessageEntity{Offset: t, Length: n, Type: "hashtag"})
			}
			t = i
		}
	}
	if t > 0 {
		e = append(e, telegram.MessageEntity{Offset: t, Length: len(v) - t, Type: "hashtag"})
	}
	return e
}
func getTarget(m *telegram.Message) (string, string) {
	switch {
	case len(m.Photo) > 0:
		i := photos(m.Photo)
		sort.Sort(i)
		return i[0].FileID, ""
	case m.Video != nil:
		return m.Video.FileID, m.Video.MimeType
	case m.Animation != nil:
		return m.Animation.FileID, m.Animation.MimeType
	case m.Document != nil:
		return m.Document.FileID, m.Document.MimeType
	}
	return "", ""
}
func (c *container) add(x context.Context, f *Forwarder, v, m, d string, o chan<- telegram.Chattable) uint8 {
	f.log.Trace(`[bot %d]: Processing ID "%s" (mime: %s) for addition..`, c.bot.Self.ID, v, m)
	i, err := loadImage(x, c.bot, v, m)
	if err == errNotImage {
		switch {
		case strings.HasSuffix(m, "/gif"):
			o <- telegram.AnimationConfig{
				Caption:  d,
				BaseFile: telegram.BaseFile{File: i, BaseChat: telegram.BaseChat{ChatID: c.recv}},
			}
		case strings.HasPrefix(m, "video/"):
			o <- telegram.VideoConfig{
				Caption:  d,
				BaseFile: telegram.BaseFile{File: i, BaseChat: telegram.BaseChat{ChatID: c.recv}},
			}
		default:
			f.log.Error(`[bot %d]: Received an invalid File "%s" (mime: %s), not forwarding it!`, c.bot.Self.ID, v, m)
			return addFailed
		}
		return addIsNotImage
	}
	if err != nil {
		f.log.Error(`[bot %d]: Received an error processing Image "%s" (mime: %s): %s!`, c.bot.Self.ID, v, m, err.Error())
		return addFailed
	}
	f.log.Debug("[bot %d]: Processing complete: %s", c.bot.Self.ID, i)
	if len(d) > 0 && strings.IndexByte(d, 0x23) >= 0 {
		strings.Split(d, "#")
	}
	k, err := c.bot.Send(
		telegram.PhotoConfig{
			BaseFile: telegram.BaseFile{
				File:     telegram.FileBytes{Name: i.Sum, Bytes: emptyJpeg},
				BaseChat: telegram.BaseChat{ChatID: c.recv, DisableNotification: true},
			},
		})
	if err != nil {
		f.log.Error("[bot %d]: Received an error adding the Placeholder Image: %s!", c.bot.Self.ID, err.Error())
		return addFailed
	}
	p := k.MessageID
	f.log.Trace(`[bot %d]: Created a Placeholder Image "%d"!`, c.bot.Self.ID, p)
	var (
		e uint64
		r *sql.Rows
	)
	if r, err = f.sql.QueryContext(x, "add", i.Average, i.Sum, c.bot.Self.ID, p); err != nil {
		f.log.Error(`[bot %d]: Received an error querying the database for "0x%X": %s!`, c.bot.Self.ID, i.Average, err.Error())
		o <- telegram.NewDeleteMessage(c.recv, p)
		return addFailed
	}
	for r.Next() {
		if err = r.Scan(&e); err != nil {
			break
		}
	}
	switch r.Close(); {
	case err != nil:
		f.log.Error("[bot %d]: Received an error scanning the query results: %s!", c.bot.Self.ID, err.Error())
		o <- telegram.NewDeleteMessage(c.recv, p)
		return addFailed
	case e != 0:
		f.log.Trace("[bot %d]: Query verified %s is already added!", c.bot.Self.ID, i)
		o <- telegram.NewDeleteMessage(c.recv, p)
		return addAlreadyExists
	}
	f.log.Debug(`[bot %d]: Updating Message "%d" with %s to the receiving Channel "%d"..`, c.bot.Self.ID, p, i, c.recv)
	_, err = c.bot.Send(telegram.EditMessageMediaConfig{
		Media: telegram.InputMediaPhoto{
			BaseInputMedia: telegram.BaseInputMedia{
				Type:            "photo",
				Media:           telegram.FileID(i.FileID),
				Caption:         d,
				ParseMode:       "markdown",
				CaptionEntities: splitTags(d),
			}},
		BaseEdit: telegram.BaseEdit{ChatID: c.recv, MessageID: p},
	})
	if err != nil {
		f.log.Error(`[bot %d]: Received an error updating the Placeholder "%d": %s!`, c.bot.Self.ID, p, err.Error())
		o <- telegram.NewDeleteMessage(c.recv, p)
		return addFailed
	}
	f.log.Debug(`[bot %d]: Update to Placeholder "%d" with %s completed!`, c.bot.Self.ID, p, i)
	return addSuccess
}
func (c *container) send(x context.Context, f *Forwarder, g *sync.WaitGroup, o <-chan telegram.Chattable) {
	f.log.Debug("[bot %d]: Starting Telegram sender thread..", c.bot.Self.ID)
	for g.Add(1); ; {
		select {
		case n := <-o:
			if _, err := c.bot.Request(n); err != nil {
				f.log.Error(`[bot %d]: Error sending Telegram message to chat: %s!`, c.bot.Self.ID, err.Error())
			}
		case <-x.Done():
			f.log.Debug("Stopping Telegram sender thread.")
			g.Done()
			return
		}
	}
}
func (c *container) delete(x context.Context, f *Forwarder, v, m string, o chan<- telegram.Chattable) bool {
	f.log.Trace(`[bot %d]: Processing ID "%s" for deletion..`, c.bot.Self.ID, v)
	i, err := loadImage(x, c.bot, v, m)
	if err != nil {
		f.log.Error(`[bot %d]: Received an error processing Image "%s" (mime: %s): %s!`, c.bot.Self.ID, v, m, err.Error())
		return false
	}
	f.log.Debug("[bot %d]: Processing complete: %s", c.bot.Self.ID, i)
	var (
		e uint64
		r *sql.Rows
	)
	if r, err = f.sql.QueryContext(x, "delete", i.Sum, c.bot.Self.ID); err != nil {
		f.log.Error(`[bot %d]: Received an error querying the database for %s: %s!`, c.bot.Self.ID, i, err.Error())
		return false
	}
	for r.Next() {
		if err = r.Scan(&e); err != nil {
			break
		}
	}
	switch r.Close(); {
	case err != nil:
		f.log.Error("[bot %d]: Received an error scanning the query results: %s!", c.bot.Self.ID, err.Error())
		return false
	case e != 0:
		f.log.Debug(`[bot %d]: Removing Message with ID "%d"..`, c.bot.Self.ID, e)
		o <- telegram.NewDeleteMessage(c.recv, int(e))
	}
	return true
}
func (c *container) receive(x context.Context, f *Forwarder, g *sync.WaitGroup, o chan<- telegram.Chattable, r <-chan telegram.Update) {
	f.log.Debug("[bot %d]: Starting Telegram receiver thread..", c.bot.Self.ID)
	for g.Add(1); ; {
		select {
		case n := <-r:
			if n.Message == nil || n.Message.Chat == nil {
				break
			}
			if !n.Message.Chat.IsPrivate() || n.Message.From.IsBot {
				break
			}
			if !c.isAuthorized(n.Message.From.ID) {
				f.log.Trace(`[bot %d]: Unauthorized user "@%s" (%d) attempted to use the bot!`, c.bot.Self.ID, n.Message.From.UserName, n.Message.From.ID)
				o <- telegram.NewMessage(n.Message.Chat.ID, "Sorry, I don't know you.")
				break
			}
			i, m := getTarget(n.Message)
			if len(i) == 0 {
				switch {
				case len(n.Message.Text) == 0:
				case strings.HasPrefix(n.Message.Text, "/del"):
					o <- telegram.NewMessage(n.Message.Chat.ID, `Use "/delete" with an image to delete it.`)
				case strings.HasPrefix(n.Message.Text, "/clear"):
					o <- telegram.NewMessage(n.Message.Chat.ID, `Removed any current cached caption!`)
					f.caps.clear(n.Message.From.ID)
				default:
					f.caps.set(n.Message.From.ID, n.Message.Text)
				}
				break
			}
			switch {
			case len(n.Message.Text) > 3 && n.Message.Text[0] == '/' && strings.HasPrefix(n.Message.Text, "/del"):
				fallthrough
			case len(n.Message.Caption) > 3 && n.Message.Caption[0] == '/' && strings.HasPrefix(n.Message.Caption, "/del"):
				f.log.Trace("[bot %d]: Received a possible delete command from %s!", c.bot.Self.ID, n.Message.From.String())
				if f.caps.clear(n.Message.From.ID); c.delete(x, f, i, m, o) {
					o <- telegram.NewMessage(n.Message.Chat.ID, "I've removed that image! (if it existed!)")
				} else {
					o <- telegram.NewMessage(n.Message.Chat.ID, "I'm sorry, but I cannot process that image.")
				}
			default:
				var (
					s  string
					ok bool
				)
				if len(n.Message.MediaGroupID) > 0 {
					if s, ok = f.groups.get(n.Message.MediaGroupID, false); !ok {
						if s, ok = f.caps.get(n.Message.From.ID, true); ok {
							f.groups.set(n.Message.MediaGroupID, s)
						} else if len(n.Message.Caption) > 0 {
							f.groups.set(n.Message.MediaGroupID, n.Message.Caption)
						}
					}
				} else if s, ok = f.caps.get(n.Message.From.ID, true); !ok {
					s = n.Message.Caption
				}
				switch c.add(x, f, i, m, s, o) {
				case addFailed:
					o <- telegram.NewMessage(n.Message.Chat.ID, "I'm sorry, but I cannot process that image.")
				case addSuccess:
					o <- telegram.MessageConfig{
						Text:                  "I've added that image!",
						BaseChat:              telegram.BaseChat{ChatID: n.Message.Chat.ID, ReplyToMessageID: 0, DisableNotification: true},
						DisableWebPagePreview: false,
					}
				case addIsNotImage:
					o <- telegram.NewMessage(
						n.Message.Chat.ID,
						"I'm sorry, I couldn't get an image hash for that, but I tried to upload it as a video instead!",
					)
				default:
					o <- telegram.MessageConfig{
						Text:                  "I've seen that image before.",
						BaseChat:              telegram.BaseChat{ChatID: n.Message.Chat.ID, ReplyToMessageID: 0, DisableNotification: true},
						DisableWebPagePreview: false,
					}
				}
			}
		case <-x.Done():
			f.log.Debug("Stopping Telegram receiver thread.")
			g.Done()
			return
		}
	}
}
