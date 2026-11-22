package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func DownloadFile(url, filePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func TranscribeAudio(filePath string) (string, error) {
	token := os.Getenv("YANDEX_TOKEN")
	folder := os.Getenv("YANDEX_FOLDER")
	audioData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("topic", "general")
	params.Set("lang", "ru-RU")
	params.Set("folderId", folder)

	endpoint := "https://stt.api.cloud.yandex.net/speech/v1/stt:recognize?" + params.Encode()

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(audioData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Api-Key "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("yandex STT error [%d]: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result string `json:"result"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal: %w, raw: %s", err, string(body))
	}

	if result.Error != "" {
		return "", fmt.Errorf("yandex error: %s", result.Error)
	}

	return result.Result, nil
}
