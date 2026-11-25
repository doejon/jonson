package jonson

// Hooks are optional interfaces that params, results and systems implement to change how
// jonson treats them. Every hook is a single method prefixed with Jonson, and every hook
// is declared in this file. Params hooks are named after the params (ValidatedParams),
// all others in Go's -er style (FieldDescriber).

// Params hooks

// ValidatedParams are parameters which have been validated before reaching the business logic.
// Make sure you do keep the validation to static data analysis and do not integrate
// any business logic validation into the validator
type ValidatedParams interface {
	JonsonValidate(validator *Validator)
}

// AllowUnknownFieldsParams are parameters that accept fields they don't declare.
// By default, a request with an unknown field is rejected, both as JSON and as form values,
// and the MCP inputSchema forbids additional properties.
type AllowUnknownFieldsParams interface {
	JonsonAllowUnknownFields()
}

// Params and result hooks

// FieldDescriber is implemented by structs whose fields are documented, e.g. by
// jonson-generate, which turns field comments into JonsonFieldDescriptions.
// Descriptions are keyed by Go field name; a `description:"..."` tag takes precedence.
// They describe the fields in the MCP inputSchema and outputSchema.
type FieldDescriber interface {
	JonsonFieldDescriptions() map[string]string
}

// Result hooks

// McpTextSummarizer is implemented by method results that summarize themselves for the model.
// A tool call returns the summary as text content and the result as structuredContent;
// without it, the text content is the result's JSON, so the result is sent twice.
type McpTextSummarizer interface {
	JonsonMcpTextContent() string
}

// System hooks

// MethodDescriber is implemented by systems whose methods are documented, e.g. by
// jonson-generate, which turns method doc comments into JonsonMethodDescriptions.
// Descriptions are keyed by endpoint name, such as "account/get-profile.v1".
// McpHandler uses them as tool descriptions.
type MethodDescriber interface {
	JonsonMethodDescriptions() map[string]string
}
