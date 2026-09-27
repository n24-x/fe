package eventbus

import "reflect"

// publishTypes returns all publish types currently registered by this client.
func (c *Client) publishTypes() []reflect.Type {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]reflect.Type, 0, len(c.pubs))
	for t := range c.pubs {
		ret = append(ret, t)
	}
	return ret
}
