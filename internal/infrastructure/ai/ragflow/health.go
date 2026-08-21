package ragflow

import "context"

type HealthChecker struct{ client *Client }

func NewHealthChecker(client *Client) *HealthChecker { return &HealthChecker{client: client} }
func (*HealthChecker) Name() string                  { return "ragflow" }
func (h *HealthChecker) Check(ctx context.Context) error {
	return h.client.Health(ctx)
}
