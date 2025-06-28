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
	"sync"
	"time"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const captionTimeout = time.Minute * 3

type caption struct {
	Tag  string
	Time time.Time
}
type imported struct {
	ID    uint64 `json:"id"`
	Bot   uint64 `json:"bot"`
	File  string `json:"file"`
	Image string `json:"image"`
}
type container struct {
	ch    chan telegram.Chattable
	key   string
	bot   *telegram.BotAPI
	recv  int64
	users []int64
}
type maps[T comparable] struct {
	v    map[T]caption
	lock sync.Mutex
}

func (c *container) stop() {
	c.bot.StopReceivingUpdates()
	close(c.ch)
	c.bot, c.ch = nil, nil
}
func (m *maps[T]) set(v T, s string) {
	m.lock.Lock()
	m.v[v] = caption{Tag: s, Time: time.Now().Add(captionTimeout)}
	m.lock.Unlock()
}
func (m *maps[int64]) clear(v int64) {
	m.lock.Lock()
	delete(m.v, v)
	m.lock.Unlock()
}
func (m *maps[T]) prune(n time.Time) {
	m.lock.Lock()
	var r []T
	for k, v := range m.v {
		if v.Time.After(n) {
			continue
		}
		r = append(r, k)
	}
	for i := range r {
		delete(m.v, r[i])
	}
	m.lock.Unlock()
}
func (f *Forwarder) tick(x context.Context) {
	t := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-x.Done():
			goto cleanup
		case n := <-t.C:
			f.log.Debug("Running Captions cleanup..")
			f.caps.prune(n)
			f.groups.prune(n)
			f.log.Debug("Captions cleanup done!")
		}
	}
cleanup:
	t.Stop()
}
func (m *maps[T]) get(v T, del bool) (string, bool) {
	m.lock.Lock()
	r, ok := m.v[v]
	if del {
		delete(m.v, v)
	}
	m.lock.Unlock()
	return r.Tag, ok
}
func (c *container) start(x context.Context, f *Forwarder, g *sync.WaitGroup) {
	r := c.bot.GetUpdatesChan(telegram.UpdateConfig{})
	c.ch = make(chan telegram.Chattable, 128)
	go c.send(x, f, g, c.ch)
	go c.receive(x, f, g, c.ch, r)
}
