package parser

// TransformParser transforms the output of one parser into another type.
type TransformParser[T, U any] struct {
	BaseParser[U]
	innerParser Parser[T]
	transform   func(T) (U, error)
}

// NewTransformParser creates a parser that transforms values from type T to type U.
func NewTransformParser[T, U any](innerParser Parser[T], transform func(T) (U, error)) *TransformParser[T, U] {
	p := &TransformParser[T, U]{
		innerParser: innerParser,
		transform:   transform,
	}

	// Set up the BaseParser with proper parse function
	p.BaseParser = BaseParser[U]{
		ParseFunc: func(s string) (U, error) {
			var zero U
			value, err := innerParser.Parse(s)
			if err != nil {
				return zero, err
			}
			return transform(value)
		},
	}

	return p
}

// ParseAndValidate parses using the inner parser and transforms the result.
func (p *TransformParser[T, U]) ParseAndValidate(s string) (U, error) {
	var zero U

	value, err := p.innerParser.ParseAndValidate(s)
	if err != nil {
		return zero, err
	}

	return p.transform(value)
}

// WithTransform creates a new parser that transforms the output of an existing parser.
func WithTransform[T, U any](parser Parser[T], transform func(T) (U, error)) Parser[U] {
	return NewTransformParser(parser, transform)
}
