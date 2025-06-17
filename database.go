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

var cleanStatements = []string{
	`DROP TABLES IF EXISTS Images`,
	`DROP PROCEDURE IF EXISTS AddImage`,
	`DROP PROCEDURE IF EXISTS DeleteImage`,
}

var setupStatements = []string{
	`CREATE TABLE IF NOT EXISTS Images(
		ImageID BIGINT(64) UNSIGNED NOT NULL PRIMARY KEY AUTO_INCREMENT,
		ImageHash BIGINT(64) UNSIGNED NOT NULL,
		ImageFileHash CHAR(128) NOT NULL,
		ImageBotID BIGINT(64) UNSIGNED NOT NULL,
		ImageMessageID BIGINT(64) UNSIGNED NOT NULL
	)`,
	`CREATE PROCEDURE IF NOT EXISTS DeleteImage(Hash1 CHAR(128), BotID BIGINT(64) UNSIGNED)
	BEGIN
		SET @image_message = COALESCE((SELECT ImageMessageID FROM Images WHERE ImageFileHash = Hash1 AND ImageBotID = BotID LIMIT 1), 0);
		IF @image_message <> 0 THEN
			DELETE FROM Images WHERE ImageMessageID = @image_message AND ImageFileHash = Hash1 AND ImageBotID = BotID;
		END IF;
		SELECT @image_message;
	END;`,
	`CREATE PROCEDURE IF NOT EXISTS AddImage(Hash1 BIGINT(64) UNSIGNED, Hash2 CHAR(128), BotID BIGINT(64) UNSIGNED, MessageID BIGINT(64) UNSIGNED)
	BEGIN
		SET @image_hash = COALESCE((SELECT ImageHash FROM Images WHERE ImageHash = Hash1 AND ImageBotID = BotID LIMIT 1), 0);
		IF @image_hash = 0 THEN
			INSERT INTO Images(ImageHash, ImageFileHash, ImageBotID, ImageMessageID) VALUES(Hash1, Hash2, BotID, MessageID);
		END IF;
		SELECT @image_hash;
	END;`,
}

var queryStatements = map[string]string{
	"add":    `CALL AddImage(?, ?, ?, ?)`,
	"delete": `CALL DeleteImage(?, ?)`,
}
