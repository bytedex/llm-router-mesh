package domain

type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type RoutingDecision struct {
	Providers []Provider
	Reason    string
}
