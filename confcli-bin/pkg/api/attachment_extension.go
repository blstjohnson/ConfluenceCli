package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
)

// UploadAttachment attaches data to a Confluence page as a named attachment,
// creating it or replacing the data of an existing same-named attachment
// (upsert). Confluence rejects a POST that would create a duplicate filename,
// so we look the attachment up first and post to its /data endpoint when it
// already exists.
//
// mimeType may be empty; when set it is recorded on the file part so
// Confluence renders the attachment inline (e.g. image/png).
func (e *PageExtension) UploadAttachment(ctx context.Context, pageID int, filename string, data []byte, mimeType string) error {
	if e.client.ReadOnly {
		return fmt.Errorf("read-only mode enabled: cannot upload attachment")
	}

	attachmentID, err := e.findAttachmentID(ctx, pageID, filename)
	if err != nil {
		return fmt.Errorf("look up existing attachment %q: %w", filename, err)
	}

	var path string
	if attachmentID != "" {
		// Replace the data of the existing attachment.
		path = fmt.Sprintf("%s/content/%d/child/attachment/%s/data", e.client.APIPrefix, pageID, attachmentID)
	} else {
		path = fmt.Sprintf("%s/content/%d/child/attachment", e.client.APIPrefix, pageID)
	}

	body, contentType, err := buildAttachmentMultipart(filename, mimeType, data)
	if err != nil {
		return fmt.Errorf("build multipart body: %w", err)
	}

	resp, err := e.client.MakeMultipartRequest(ctx, "POST", path, nil, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload attachment %q failed with status %d: %s", filename, resp.StatusCode, string(msg))
	}
	return nil
}

// findAttachmentID returns the id of an attachment with the given filename on
// the page, or "" when none exists.
func (e *PageExtension) findAttachmentID(ctx context.Context, pageID int, filename string) (string, error) {
	params := url.Values{}
	params.Add("filename", filename)

	path := fmt.Sprintf("%s/content/%d/child/attachment", e.client.APIPrefix, pageID)
	resp, err := e.client.MakeRequest(ctx, "GET", path, params, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(msg))
	}

	var result struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Results) == 0 {
		return "", nil
	}
	return result.Results[0].ID, nil
}

// buildAttachmentMultipart assembles a multipart/form-data body with a single
// "file" part (and minorEdit=true so re-uploads don't spam watchers). It
// returns the body reader and the Content-Type header value carrying the
// generated boundary.
func buildAttachmentMultipart(filename, mimeType string, data []byte) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	if mimeType != "" {
		h.Set("Content-Type", mimeType)
	}
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}

	if err := w.WriteField("minorEdit", "true"); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
