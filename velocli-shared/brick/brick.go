package brick

type Brick struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Version          string         `json:"version"`
	EncryptedPayload []byte         `json:"encrypted_payload"`
	Variables        []VariableSpec `json:"variables"`
}

type VariableSpec struct {
	Key      string `json:"key"`
	Required bool   `json:"required"`
}
