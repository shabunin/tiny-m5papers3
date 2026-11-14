#!/usr/bin/env python3
"""Talk to the launcher (examples/launcher) over USB serial.

    tools/papers3.py time                 set the RTC to this PC's local time
    tools/papers3.py shot [file.png]      save the device's frame buffer as PNG
    tools/papers3.py tap X Y              touch the screen (portrait coordinates)
    tools/papers3.py drag X0 Y0 X1 Y1     swipe
    tools/papers3.py open NAME            open an app ("home", "paint", ...)
    tools/papers3.py status               print board status
    tools/papers3.py log [SECONDS]        print the serial output

Needs pyserial (and Pillow for shot). The port defaults to /dev/ttyACM0
(set PAPERS3_PORT to change it).
"""
import os
import sys
import time
from datetime import datetime

import serial

PORT = os.environ.get("PAPERS3_PORT", "/dev/ttyACM0")


def open_port():
    # Leave DTR/RTS as the kernel sets them on open (asserted): releasing
    # them is the USB-Serial-JTAG reset sequence and would reboot the board.
    # write_timeout: a board that is asleep or stuck does not take input;
    # fail instead of blocking forever.
    return serial.Serial(PORT, 115200, timeout=0.2, write_timeout=2)


def send(s, line):
    try:
        s.write((line + "\n").encode())
        s.flush()
    except serial.SerialTimeoutException:
        sys.exit(f"{PORT}: the board does not read (asleep or stuck?)")


def read_for(s, seconds):
    end = time.time() + seconds
    buf = b""
    while time.time() < end:
        buf += s.read(4096)
    return buf.decode(errors="replace")


def shot(s, path):
    from PIL import Image

    s.reset_input_buffer()
    send(s, "shot")
    rows, rot, end = [], None, time.time() + 30
    pending = b""
    while time.time() < end:
        pending += s.read(65536)
        *lines, pending = pending.split(b"\n")
        for ln in lines:
            ln = ln.strip().decode(errors="replace")
            if ln.startswith("SHOT"):
                rot, rows = int(ln.split()[1]), []
            elif ln == "END" and rot is not None:
                end = 0
                break
            elif rot is not None and len(ln) == 960:
                rows.append(bytes.fromhex(ln))
        if end == 0:
            break
    if len(rows) != 540:
        sys.exit(f"shot: got {len(rows)} rows")
    px = bytearray(960 * 540)
    for y, row in enumerate(rows):
        for i, b in enumerate(row):
            px[y * 960 + 2 * i] = (b >> 4) * 17
            px[y * 960 + 2 * i + 1] = (b & 15) * 17
    img = Image.frombytes("L", (960, 540), bytes(px))
    img = {0: img.transpose(Image.Transpose.ROTATE_270), 1: img,
           2: img.transpose(Image.Transpose.ROTATE_90),
           3: img.transpose(Image.Transpose.ROTATE_180)}[rot]
    img.save(path)
    print("saved", path, img.size)


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    cmd, args = sys.argv[1], sys.argv[2:]
    s = open_port()
    if cmd == "shot":
        shot(s, args[0] if args else "papers3.png")
    elif cmd == "time":
        now = datetime.now()
        # Send at the start of a second so the RTC is not ~1 s behind.
        time.sleep(1 - now.microsecond / 1e6)
        send(s, "time " + datetime.now().strftime("%Y-%m-%d %H:%M:%S"))
        print(read_for(s, 1.5), end="")
    elif cmd == "log":
        print(read_for(s, float(args[0]) if args else 10), end="")
    elif cmd in ("tap", "drag", "open", "status", "beep"):
        send(s, " ".join([cmd] + args))
        print(read_for(s, 2.5), end="")
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
