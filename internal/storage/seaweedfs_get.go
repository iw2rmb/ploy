package storage

import (
	"fmt"
	"io"
	"net/http"
)

func (c *SeaweedFSClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("failed to get object: %s", resp.Status)
	}
	return resp.Body, nil
}
