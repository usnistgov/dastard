package lancero

const (
	colRegisterBase int64 = 0x100                  // collector register base address
	colRegisterIDV  int64 = colRegisterBase + 0x00 // collector register offset for id and version numbers
	colRegisterCtrl int64 = colRegisterBase + 0x04 // collector register offset for control register
	colRegisterLP   int64 = colRegisterBase + 0x08 // collector register offset for line sync period
	colRegisterDD   int64 = colRegisterBase + 0x0c // collector register offset for data delay
	colRegisterMask int64 = colRegisterBase + 0x10 // collector register offset for channel masks
	colRegisterFL   int64 = colRegisterBase + 0x14 // collector register offset for frame length

	bitsCtrlRun uint32 = 0x1 // collector control register bit for running acquisition
	bitsCtrlSim uint32 = 0x2 // collector control register bit for sim mode
)

// collector is the interface to the component that combines several optical
// fibers onto one serial stream. Must be started and stopped; also controls
// fiber-reading mode vs simulated data mode.
type collector struct {
	device    *lanceroDevice
	simulated bool
}

func (c *collector) configure(linePeriod, dataDelay, channelMask, frameLength uint32) error {
	if err := c.device.writeRegister(colRegisterLP, linePeriod); err != nil {
		return err
	}
	if err := c.device.writeRegister(colRegisterDD, dataDelay); err != nil {
		return err
	}
	if err := c.device.writeRegister(colRegisterMask, channelMask); err != nil {
		return err
	}
	if err := c.device.writeRegisterFlush(colRegisterFL, frameLength); err != nil {
		return err
	}
	return nil
}

func (c *collector) start(simulate bool) error {
	c.simulated = simulate
	runCmd := bitsCtrlRun
	if simulate {
		// Start the simulator
		if err := c.device.writeRegisterFlush(colRegisterCtrl, bitsCtrlSim); err != nil {
			return err
		}
		runCmd |= bitsCtrlSim
	}
	// Now enable the clock (with sim still enabled or not)
	if err := c.device.writeRegisterFlush(colRegisterCtrl, runCmd); err != nil {
		return err
	}
	return nil
}

func (c *collector) stop() error {
	if c.simulated {
		if err := c.device.writeRegisterFlush(colRegisterCtrl, bitsCtrlSim); err != nil {
			return err
		}
	}
	c.simulated = false
	return c.device.writeRegisterFlush(colRegisterCtrl, 0)
}
