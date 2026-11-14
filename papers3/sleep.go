package papers3

// Wake tells what ended a sleep (LightSleep) or why the program started
// (Status.Wake).
type Wake uint8

const (
	// WakeNone: a normal start (power button, reset, flashing, crash), or a
	// LightSleep that the hardware refused because a wake source was already
	// active.
	WakeNone Wake = iota
	// WakeTimer: the sleep time was over (LightSleep, or the restart after
	// DeepSleep).
	WakeTimer
	// WakeTouch: the touch panel was touched during LightSleep.
	WakeTouch
	// WakeAlarm: the board was switched off by PowerOffFor and the RTC timer
	// switched it on again.
	WakeAlarm
)

func (w Wake) String() string {
	switch w {
	case WakeTimer:
		return "timer"
	case WakeTouch:
		return "touch"
	case WakeAlarm:
		return "alarm"
	}
	return "none"
}

// Retained words survive DeepSleep (they live in the RTC domain, which keeps
// running), but not PowerOff or a power cut. They read 0 after power-on.
const RetainedWords = 2
