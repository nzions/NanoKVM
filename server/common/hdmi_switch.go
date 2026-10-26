package common

// HdmiSwitch is the outcome of asking libkvm to switch HDMI capture.
type HdmiSwitch int

const (
	// HdmiSwitched: libkvm switched the LT6911 reset line.
	HdmiSwitched HdmiSwitch = iota
	// HdmiNoResetLine: this board (the Cube) has no software-driven reset
	// line, so there was nothing to switch. Not an error.
	HdmiNoResetLine
	// HdmiSwitchFailed: libkvm tried and failed, or is not there.
	HdmiSwitchFailed
)
