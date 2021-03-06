package imap

import (
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap-move"
	"github.com/emersion/go-imap-uidplus"
	"time"
)

// This function is responsive for toggling of IDLE
func (c *Client) idleOnInbox() {
	select {
	case <-time.After(5 * time.Second):
		// Wait for 5 seconds before IDLE mode entering.
	case <-c.idlerInterrupt:
		// If during these 5 seconds we received interrupt request
		// - don't enter IDLE mode, ack request and return.
		c.idlerInterrupt <- true
		return
	}

	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	_, err := c.ensureSelected("INBOX", true)
	if err != nil {
		c.Logger.Println("Mailbox selection failed, not entering IDLE mode:", err)
		if err := c.recoverIdler(); err != nil {
			// ehh...
		}
		go c.idleOnInbox()
		return
	}
	defer c.cl.Close()

	// Used to signal error occured during IDLE.
	idleChan := make(chan error, 1)
	// Used to stop IDLE forcefully.
	idleStop := make(chan struct{})

	c.Logger.Println("Entering IDLE mode...")

	supported, err := c.idle.SupportIdle()
	if err != nil {
		c.Logger.Println("Capability query failed, not entering IDLE mode:", err)
	}
	if !supported {
		c.Logger.Println("No IMAP IDLE support, falling back to polling.")
	}

	// Disable regular I/O timeout in IDLE mode.
	c.cl.Timeout = time.Duration(0)

	go func() {
		// Setting very small "heartbeat" delay because some NATs and mail
		// servers are really stupid to drop IDLE'ing connections.
		idleChan <- c.idle.IdleWithFallback(idleStop, 60*time.Second)
	}()

	for {
		select {
		case <-c.idlerInterrupt:
			c.Logger.Println("Exiting IDLE mode...")
			close(idleStop)
		case idleErr := <-idleChan:
			if idleErr != nil {
				if idleErr.Error() == "imap: connection closed" {
					if err := c.recoverIdler(); err != nil {
						c.Logger.Println("Connection recovery during idle failed, bailing out:", err)
					}
					return
				}
				c.Logger.Println("Idle error:", idleErr)
			}
			c.idlerInterrupt <- true

			// Re-enable regular I/O timeout.
			c.cl.Timeout = 30 * time.Second

			return
		}
	}

}

func (c *Client) stopIdle() {
	// Ask idle goroutine to exit.
	c.idlerInterrupt <- true
	// Wait for ack to make sure we done with idling before doing regular requests.
	<-c.idlerInterrupt
}

func (c *Client) resumeIdle() {
	// TODO: Is constant goroutine restarting expensive?
	go c.idleOnInbox()
}

// recoverIdler is called from idleOnInbox in attempt to reconnect after
// unexpected connection close from server (this happens pretty often, because
// connection is actually idle when client is in IDLE mode).
//
// This task is also a special case of c.Reconnect function because IDLE thing
// on it's own is a very special snowflake which needs very careful handling.
func (c *Client) recoverIdler() error {
	c.Logger.Println("Lost connection during IDLE, trying to recover...")
	c.updatesDispatcherStop <- true
	<-c.updatesDispatcherStop

	c.cl = nil
	var err error
	c.cl, err = connect(c.LastConfig)
	if err != nil {
		return err
	}
	c.idle = idle.NewClient(c.cl)
	c.move = move.NewClient(c.cl)
	c.uidplus = uidplus.NewClient(c.cl)

	if err := c.Auth(c.LastConfig); err != nil {
		return err
	}

	if _, err := c.ensureSelected("INBOX", true); err != nil {
		return err
	}

	go c.updatesWatch()
	return nil
}
