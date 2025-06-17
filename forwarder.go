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
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/PurpleSec/logx"
	"github.com/PurpleSec/mapper"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const captionTimeout = time.Minute * 5

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

// Forwarder is a struct that contains the threads and config values that can be
// used to run the Forwarder Telegram bot.
//
// Use the 'New' function to properly create a Forwarder.
type Forwarder struct {
	log      logx.Log
	sql      *mapper.Map
	bots     []*container
	lock     sync.Mutex
	cancel   context.CancelFunc
	captions map[int64]caption
}
type container struct {
	ch    chan telegram.Chattable
	key   string
	bot   *telegram.BotAPI
	recv  int64
	users []int64
}

func (c *container) stop() {
	c.bot.StopReceivingUpdates()
	close(c.ch)
	c.bot, c.ch = nil, nil
}

// Run will start the main Forwarder process and all associated threads. This
// function will block until an interrupt signal is received.
//
// This function returns any errors that occur during shutdown.
func (f *Forwarder) Run() error {
	var (
		o = make(chan os.Signal, 1)
		x context.Context
		g sync.WaitGroup
	)
	signal.Notify(o, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	x, f.cancel = context.WithCancel(context.Background())
	f.log.Info("Forwarder Started, spinning up Bot threads..")
	for i := range f.bots {
		f.log.Debug("Starting bot %d..", i)
		f.bots[i].start(x, f, &g)
	}
	go f.tick(x)
	for {
		select {
		case <-o:
			goto cleanup
		case <-x.Done():
			goto cleanup
		}
	}
cleanup:
	signal.Stop(o)
	f.cancel()
	for i := range f.bots {
		f.log.Debug("Stopping Bot %d..", i)
		f.bots[i].stop()
	}
	g.Wait()
	close(o)
	return f.sql.Close()
}

// Import will attempt to import the data contained in the supplied filepath as
// a JSON export using the "import.py" tool.
func (f *Forwarder) Import(s string) error {
	f.log.Info(`Attempting to import Messages from file "%s"..`, s)
	v, err := os.Open(s)
	if err != nil {
		return errors.New(`cannot open "` + s + `": ` + err.Error())
	}
	var e []imported
	err = json.NewDecoder(v).Decode(&e)
	if v.Close(); err != nil {
		return errors.New(`cannot parse "` + s + `": ` + err.Error())
	}
	f.log.Info(`Found "%d" records in "%s"..`, len(e), s)
	var (
		o    = make(chan os.Signal, 1)
		x, y = context.WithCancel(context.Background())
	)
	signal.Notify(o, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for i := range e {
		if e[i].Bot == 0 || e[i].ID == 0 || len(e[i].File) == 0 || len(e[i].Image) == 0 {
			f.log.Warning(`Skipping invalid record at "%d" in "%s".`, i, s)
			continue
		}
		var h uint64
		if h, err = strconv.ParseUint(e[i].Image, 16, 64); err != nil || h == 0 {
			f.log.Warning(`Skipping invalid record at "%d" with bad Image hash "%s" in "%s".`, i, e[i].Image, s)
			continue
		}
		if _, err = f.sql.ExecContext(x, "add", h, e[i].File, e[i].Bot, e[i].ID); err != nil {
			err = errors.New(`cannot import record "` + strconv.Itoa(i) + `" in "` + s + `": ` + err.Error())
			break
		}
	}
	if y(); err != nil {
		f.sql.Close()
	}
	return err
}
func (f *Forwarder) tick(x context.Context) {
	t := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-x.Done():
			goto cleanup
		case n := <-t.C:
			f.log.Debug("Running Captions cleanup..")
			f.lock.Lock()
			var r []int64
			for k, v := range f.captions {
				if v.Time.After(n) {
					continue
				}
				r = append(r, k)
			}
			for i := range r {
				delete(f.captions, r[i])
			}
			f.log.Debug("Captions cleanup done! %d were removed.", len(r))
			f.lock.Unlock()
		}
	}
cleanup:
	t.Stop()
}

// New returns a new Forwarder instance based on the passed config file path. This function will preform any
// setup steps needed to start the Forwarder. Once complete, use the 'Run' function to actually start the Forwarder.
//
// This function allows for specifying the option to clear the database before starting.
func New(s string, empty bool) (*Forwarder, error) {
	var c config
	j, err := os.ReadFile(s)
	if err != nil {
		return nil, errors.New(`reading config "` + s + `" failed: ` + err.Error())
	}
	if err = json.Unmarshal(j, &c); err != nil {
		return nil, errors.New(`parsing config "` + s + `" failed: ` + err.Error())
	}
	if err = c.check(); err != nil {
		return nil, err
	}
	l := logx.Multiple(logx.Console(logx.Level(c.Log.Level)))
	if len(c.Log.File) > 0 {
		f, err2 := logx.File(c.Log.File, logx.Append, logx.Level(c.Log.Level))
		if err2 != nil {
			return nil, errors.New(`log file "` + c.Log.File + `" creation failed: ` + err2.Error())
		}
		l.Add(f)
	}
	z := make([]*container, 0, len(c.Bots))
	for i := range c.Bots {
		b, err := telegram.NewBotAPIWithClient(c.Bots[i].Key, "https://api.telegram.org/bot%s/%s", &http.Client{
			Transport: &http.Transport{
				Proxy:             http.ProxyFromEnvironment,
				MaxIdleConns:      256,
				ForceAttemptHTTP2: false,
			},
		})
		if err != nil {
			return nil, errors.New("bot " + strconv.Itoa(i) + ": login failed: " + err.Error())
		}
		z = append(z, &container{bot: b, key: c.Bots[i].Key, recv: c.Bots[i].Channel, users: c.Bots[i].Users})
	}
	if len(z) == 0 {
		return nil, errors.New("no telegram accounts")
	}
	d, err := sql.Open(
		"mysql",
		c.Database.Username+":"+c.Database.Password+"@"+c.Database.Server+"/"+c.Database.Name+"?multiStatements=true&interpolateParams=true",
	)
	if err != nil {
		return nil, errors.New(`database connection "` + c.Database.Server + `" failed: ` + err.Error())
	}
	if err = d.Ping(); err != nil {
		return nil, errors.New(`database connection "` + c.Database.Server + `" failed: ` + err.Error())
	}
	m := mapper.New(d)
	if d.SetConnMaxLifetime(c.Database.Timeout); empty {
		if err = m.Batch(cleanStatements); err != nil {
			m.Close()
			return nil, errors.New("clean up failed: " + err.Error())
		}
	}
	if err = m.Batch(setupStatements); err != nil {
		m.Close()
		return nil, errors.New("database schema setup failed: " + err.Error())
	}
	if err = m.Extend(queryStatements); err != nil {
		m.Close()
		return nil, errors.New("database schema extend failed: " + err.Error())
	}
	return &Forwarder{
		sql:      m,
		log:      l,
		bots:     z,
		captions: make(map[int64]caption),
	}, nil
}
func (c *container) start(x context.Context, f *Forwarder, g *sync.WaitGroup) {
	r := c.bot.GetUpdatesChan(telegram.UpdateConfig{})
	c.ch = make(chan telegram.Chattable, 128)
	go c.send(x, f, g, c.ch)
	go c.receive(x, f, g, c.ch, r)
}
