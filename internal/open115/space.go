package open115

import "context"

func (c *Client) Space(ctx context.Context) (Space, error) {
	return c.api.Space(ctx)
}
