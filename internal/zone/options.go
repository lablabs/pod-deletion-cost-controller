package zone

type config struct {
	spreadByAnnotation string
}

type Option func(*config) error

func WithSpreadByAnnotation(annotation string) Option {
	return func(c *config) error {
		c.spreadByAnnotation = annotation
		return nil
	}
}
