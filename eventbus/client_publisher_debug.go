package eventbus

import "reflect"

// publishTypes 返回本客户端当前登记的全部发布类型（统计/调试用，Debugger 接入）。
func (c *Client) publishTypes() []reflect.Type {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]reflect.Type, 0, len(c.pubs))
	for t := range c.pubs {
		ret = append(ret, t)
	}
	return ret
}
