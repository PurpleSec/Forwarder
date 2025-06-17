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
	"errors"
	"strconv"
	"time"

	// Import for the Golang MySQL driver
	_ "github.com/go-sql-driver/mysql"
)

// Defaults is a string representation of a JSON formatted default configuration
// for a Forwarder instance.
const Defaults = `{
	"db": {
		"host": "tcp(localhost:3306)",
		"user": "forwarder_user",
		"timeout": 180000000000,
		"password": "password",
		"database": "forwarder_db"
	},
	"log": {
		"file": "forwarder.log",
		"level": 2
	},
	"bots": [
		{
			"channel_id": 0,
			"telegram_key": "",
			"authorized_users": [
				0,
				1
			]
		}
	]
}
`

type log struct {
	File  string `json:"file"`
	Level int    `json:"level"`
}
type bot struct {
	Key     string  `json:"telegram_key"`
	Users   []int64 `json:"authorized_users"`
	Channel int64   `json:"channel_id"`
}
type config struct {
	Log      log      `json:"log"`
	Bots     []bot    `json:"bots"`
	Database database `json:"db"`
}
type database struct {
	Name     string        `json:"database"`
	Server   string        `json:"host"`
	Timeout  time.Duration `json:"timeout"`
	Username string        `json:"user"`
	Password string        `json:"password"`
}

func (c *config) check() error {
	if len(c.Database.Name) == 0 {
		return errors.New("missing database name")
	}
	if len(c.Database.Server) == 0 {
		return errors.New("missing database server")
	}
	if len(c.Database.Username) == 0 {
		return errors.New("missing database username")
	}
	if c.Database.Timeout == 0 {
		c.Database.Timeout = time.Minute * 3
	}
	for i := range c.Bots {
		if c.Bots[i].Channel == 0 {
			return errors.New("bot " + strconv.Itoa(i) + ": missing channel_id")
		}
		if len(c.Bots[i].Key) == 0 {
			return errors.New("bot " + strconv.Itoa(i) + ": missing telegram_key")
		}
	}
	return nil
}
