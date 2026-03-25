package transforms

// Pipeline executes a sequence of transforms in order.
type Pipeline struct {
	transforms []Transform
}

// NewPipeline creates a pipeline from the given transforms.
func NewPipeline(transforms ...Transform) *Pipeline {
	return &Pipeline{transforms: transforms}
}

// Run executes all transforms in order on the given context.
// If any transform returns an error, execution stops and the error is returned.
func (p *Pipeline) Run(ctx *TransformContext) error {
	for _, t := range p.transforms {
		if err := t.Apply(ctx); err != nil {
			return &TransformError{TransformName: t.Name(), Err: err}
		}
	}
	return nil
}

// Append adds transforms to the end of the pipeline.
func (p *Pipeline) Append(transforms ...Transform) {
	p.transforms = append(p.transforms, transforms...)
}

// Len returns the number of transforms in the pipeline.
func (p *Pipeline) Len() int {
	return len(p.transforms)
}
