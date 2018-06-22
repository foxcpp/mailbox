package core

func (c *Client) EncryptUsingMaster(blob []byte) []byte {
	return blob
}

func (c *Client) DecryptUsingMaster(blob []byte) ([]byte, error) {
	return blob, nil
}
