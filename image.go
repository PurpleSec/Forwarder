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
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/corona10/goimagehash"
	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var errNotImage = errors.New("not an image")

type reader struct {
	h hash.Hash
	r io.ReadCloser
}
type fileData struct {
	Sum     string
	FileID  string
	Average uint64
}

func fnv(n string) uint32 {
	h := uint32(2166136261)
	for i := range n {
		h *= 16777619
		h ^= uint32(n[i])
	}
	return h
}
func (f fileData) String() string {
	return fmt.Sprintf("Image(%x/%x/%x)", fnv(f.FileID), fnv(f.Sum), f.Average)
}
func (fileData) NeedsUpload() bool {
	return false
}
func (f fileData) SendData() string {
	return f.FileID
}
func (r *reader) Read(b []byte) (int, error) {
	n, err := r.r.Read(b)
	r.h.Write(b[0:n])
	return n, err
}
func (fileData) UploadData() (string, io.Reader, error) {
	return "", nil, os.ErrInvalid
}
func loadImage(x context.Context, bot *telegram.BotAPI, id string, mime string) (fileData, error) {
	if len(mime) > 0 && !strings.HasPrefix(mime, "image/") {
		return fileData{FileID: id}, errNotImage
	}
	f, err := bot.GetFile(telegram.FileConfig{FileID: id})
	if err != nil {
		return fileData{}, err
	}
	var (
		z, _ = http.NewRequestWithContext(x, "GET", fmt.Sprintf(telegram.FileEndpoint, bot.Token, f.FilePath), nil)
		d    *http.Response
	)
	if d, err = bot.Client.Do(z); err != nil {
		return fileData{}, err
	}
	var (
		r = reader{h: sha512.New(), r: d.Body}
		i image.Image
	)
	switch mime {
	case "image/png":
		i, err = png.Decode(&r)
	default:
		i, err = jpeg.Decode(&r)
	}
	if d.Body.Close(); err != nil {
		return fileData{FileID: id}, errNotImage
	}
	h, err := goimagehash.PerceptionHash(i)
	if err != nil {
		return fileData{}, err
	}
	return fileData{
		Sum:     hex.EncodeToString(r.h.Sum(nil)),
		FileID:  id,
		Average: h.GetHash(),
	}, nil
}
