#!/usr/bin/python3
#
# Copyright (c) 2025 PurpleSec
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published
# by the Free Software Foundation, either version 3 of the License, or
# any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
#

from PIL import Image
from io import BytesIO
from hashlib import sha512
from imagehash import phash
from json import loads, dumps
from traceback import format_exc
from sys import stderr, exit, argv
from argparse import ArgumentParser
from telethon.sync import TelegramClient
from os.path import expanduser, expandvars, exists


USAGE = """Forwarder Telegram Bot Channel Import Tool

Usage: {bin} [-s file] [--no-state] [-f file] [-b id] [-o file] -i <app_id> -n <app_hash> <channel_name>

Positional Arguments:
    channel_name               Name of the Channel to use.

Required Arguments:
    -i           <app_id>      Telegram API "app_id" value.
    --app-id
    -n           <app_hash>    Telegram API "app_hash" value.
    --app-hash

Optional Arguments:
    -s           <file>        Name of the saved session state file. Used to prevent
    --state                     needing to supply credentials every time used.
    -o           <output_file> Path to save the import file to. Defaults to
    --output                    "forwarder_import.json".
    -c           <config_file> Path to the Forwarder config file. Used to
    --config                    resolve BotIDs to Channels. This is ignored if
                                the "-b"/"--bot" argument is used.
    -b           <bot_id>      Numerical ID of the Bot assigned to the Channel
    --bot                       import. Use this to assign the BotID directly
                                instead
    --state                     needing to supply credentials every time used.
    --no-state                 Prevent using a state file. By default if no "-f/--state"
                                argument is provided, the value "import" will be used.
                                pass this flag to prevent this default state file from
                                being used. Overrides any "-f/--state" argument.

The app_id and app_hash values can be gerenerated via the Telegram API page at
https://my.telegram.org

On first run or when no state file is used, you will be asked for Telegram login
credentials. You may use the credentials of any account that has delete permission
for media in the target Telegram channel.

NOTE: YOU CANNOT USE BOT TOKENS FOR THIS!! Bots do NOT have the ability to list
channels.
"""


def _main():
    p = ArgumentParser()
    p.print_help = _usage
    p.print_usage = _usage
    p.add_argument(
        "-s",
        "--state",
        type=str,
        dest="state",
        action="store",
        default="import",
        required=False,
    )
    p.add_argument(
        "-o",
        "--output",
        type=str,
        dest="output",
        action="store",
        default="forwarder_import.json",
        required=False,
    )
    p.add_argument(
        "-c",
        "--config",
        type=str,
        dest="config",
        action="store",
        default="forwarder.json",
        required=False,
    )
    p.add_argument(
        "-b",
        "--bot",
        type=int,
        dest="bot",
        action="store",
        default=0,
        required=False,
    )
    p.add_argument(
        "-i",
        "--app-id",
        type=int,
        dest="app_id",
        action="store",
        required=True,
    )
    p.add_argument(
        "-n",
        "--app-hash",
        type=str,
        dest="app_hash",
        action="store",
        required=True,
    )
    p.add_argument(
        "--no-state",
        dest="no_state",
        action="store_true",
        required=False,
    )
    p.add_argument(
        type=str,
        dest="name",
        nargs=1,
        action="store",
    )
    a = p.parse_args()
    del p
    if len(a.name) == 0:
        raise ValueError('"name" cannot be empty')
    if len(a.output) == 0:
        raise ValueError('"output" cannot be empty')
    n = a.name[0]
    if not isinstance(a.name[0], str) or len(a.name[0]) == 0:
        raise ValueError('"name" cannot be empty')
    if not isinstance(a.app_hash, str) or len(a.app_hash) == 0:
        raise ValueError("app_hash cannot be empty")
    if a.no_state:
        s = None
    else:
        s = a.state
    d = expandvars(expanduser(a.output))
    with TelegramClient(s, a.app_id, a.app_hash) as x:
        c = _find_channel(x, n)
        if c is None:
            raise ValueError(f'no channel with name "{a.name}" found')
        print(f'Found Channel "{c.name}" with ID {c.id}..')
        if a.bot != 0:
            b = a.bot
        else:
            b = _find_bot(a, c.id)
        if not isinstance(b, int) or b == 0:
            raise ValueError("no valid BotID found")
        _generate_imports(x, c, d, b)
        del c
    del s, d, n


def _usage(*_):
    print(USAGE.format(bin=argv[0]))
    exit(2)


def _find_bot(args, channel):
    if not isinstance(args.config, str) or len(args.config) == 0:
        raise ValueError('"config" is invalid')
    c = expandvars(expanduser(args.config))
    if not exists(c):
        raise ValueError('"config" must be a valid file or a bot ID must be specified')
    with open(c) as f:
        v = loads(f.read())
    del c
    if not isinstance(v, dict) or len(v) == 0:
        raise ValueError(f'config data in "{args.config}" is invalid')
    for i in v.get("bots", list()):
        if not isinstance(i, dict) or len(i) == 0:
            continue
        if "telegram_key" in i and i.get("channel_id") == channel:
            c = TelegramClient(None, args.app_id, args.app_hash).start(
                bot_token=i["telegram_key"]
            )
            try:
                return c.get_me().id
            finally:
                del c
    return None


def _find_channel(client, name):
    for i in client.get_dialogs():
        if not i.is_channel or i.is_group:
            continue
        if i.name != name:
            continue
        return i
    return None


def _generate_imports(client, channel, output, bot):
    e, n = dict(), 0
    for i in client.iter_messages(channel.id, reverse=True):
        if n % 20 == 0:
            print(f"Checking message {n}..")
        n += 1
        if i.file is None:
            continue
        b = i.download_media(file=bytes, thumb=None)
        z = sha512(usedforsecurity=False)
        z.update(b)
        d = z.hexdigest()
        del z
        try:
            with Image.open(BytesIO(b)) as f:
                h = str(phash(f))
        except Exception:
            print(f"Skipping non-image {i.id}..")
            continue
        if h in e:
            print(f"Duplicate of {h} detected in Message {i.id}, skipping it..")
            continue
        e[h] = {"image": h, "file": d, "bot": bot, "id": i.id}
        del b, d, h
    with open(output, "w") as f:
        f.write(dumps(list(e.values())))
    del e


if __name__ == "__main__":
    try:
        _main()
    except Exception as err:
        print(f"Error during runtime: {err}!\n{format_exc(limit=7)}", file=stderr)
        exit(1)
