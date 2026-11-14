package ui

// BookText is the default text of the Reader: a short guide to this device
// and its software.
const BookText = `# Paper that thinks

You are reading this on a sheet of electronic paper, driven by a program written in Go and compiled by TinyGo. No operating system is running underneath: when the chip starts, it jumps straight into Go code, which talks to the hardware registers itself.

The screen keeps showing this page even if the power goes away. That is the whole point of e-paper: it only needs energy to change, never to stay.

# How the ink works

Every pixel is a thin layer of tiny capsules. Each capsule holds black and white particles floating in a clear liquid. The black particles carry one electric charge, the white ones the opposite charge.

Put a voltage across a pixel and the particles move: the white ones rise to the surface and the pixel looks white, or the black ones do and it looks black. Stop in between and you get a gray. Take the voltage away and the particles simply stay where they are.

# A screen without a controller

This panel has no brain of its own. The microcontroller scans it like an old television: row after row, 540 rows per frame, it tells each of the 518,400 pixels whether to move towards black, towards white, or to stay put.

One frame takes about fifteen milliseconds, and one frame only nudges the particles a little. Turning a white pixel black takes several frames in a row; landing exactly on one of the sixteen gray levels takes a carefully timed sequence of pushes in both directions. Such a sequence is called a waveform.

# Why the screen flashes

The particles remember where they were, but the controller does not know that precisely. Small errors add up and leave faint shadows of earlier pictures, called ghosting.

The cure is to shake the pixels first: drive them fully black, fully white, black and white again, so that every pixel starts from a known state. That is the flash you see during a high quality update. A fast update skips the flash and only knows black and white, which is good enough for a pen stroke or a ticking clock.

# Only what changed

This program remembers which pixels changed since they were last sent to the panel. An update only drives those pixels and leaves all others alone, so a new minute on the clock flashes just the digits that changed.

The rows are pushed out by the chip's LCD peripheral with DMA, sixteen million bus cycles per second, while the processor prepares the next row. The two must finish in step, so the code that prepares a row was tuned until it fits into the fifteen microseconds the hardware needs to send the previous one.

# What else is inside

Besides the display, the board has a touch panel that can follow five fingers, a clock chip that keeps the time while the device is off, a motion sensor that knows which way is down, a battery with a charger, a buzzer and a small green light.

All of them are reachable from Go. The launcher you came from is a few hundred lines of it, and the same code also runs on a PC, where the display is simulated and every screen can be saved as a picture.

# Things to try

Draw with your finger in Paint and press Sharpen afterwards. Zoom into the fractal by tapping on its edge. Let Life run for a while, then pause it. Put the device on a table and watch the Level. Look at the Grays screen with each waveform and compare.

Tap the left side of this page to go back, the right side to go forward, or Home to leave.`
