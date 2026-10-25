package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/r0madeus/veto/internal/utils"
)

const (
	MediaTypeManifestV2  = "application/vnd.docker.distribution.manifest.v2+json"
	MediaTypeOCIIndex    = "application/vnd.oci.image.index.v1+json"
	MediaTypeOCIManifest = "application/vnd.oci.image.manifest.v1+json"
)

// OCIClient handles interactions with OCI registries.
type OCIClient struct {
	Client *http.Client
}

func NewOCIClient() *OCIClient {
	return &OCIClient{
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Pull downloads an OCI artifact (recipe) to the destination directory.
// ref format: oci://registry/repo:tag
func (c *OCIClient) Pull(ref, destDir string) error {
	registry, repo, tag, err := parseRef(ref)
	if err != nil {
		return err
	}

	// 1. Get Manifest
	manifest, err := c.getManifest(registry, repo, tag)
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	// 2. Find Layer (Assuming single layer for recipes or taking the first one)
	if len(manifest.Layers) == 0 {
		return fmt.Errorf("no layers found in manifest")
	}
	layerDigest := manifest.Layers[0].Digest

	// 3. Download Layer Blob
	blobStream, err := c.getBlob(registry, repo, layerDigest)
	if err != nil {
		return fmt.Errorf("failed to download blob: %w", err)
	}
	defer blobStream.Close()

	// 4. Extract
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	return utils.ExtractTarGz(blobStream, destDir)
}

func (c *OCIClient) getManifest(registry, repo, tag string) (*ManifestV2, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Accept headers for OCI/Docker manifests
	req.Header.Set("Accept", fmt.Sprintf("%s, %s, %s", MediaTypeManifestV2, MediaTypeOCIManifest, MediaTypeOCIIndex))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication required (private registries not yet supported)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request failed: %s", resp.Status)
	}

	var manifest ManifestV2
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *OCIClient) getBlob(registry, repo, digest string) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repo, digest)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("blob request failed: %s", resp.Status)
	}

	return resp.Body, nil
}

// Basic struct for Manifest V2 parsing
type ManifestV2 struct {
	SchemaVersion int `json:"schemaVersion"`
	Layers        []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func parseRef(ref string) (registry, repo, tag string, err error) {
	// Expected: oci://registry/repo:tag
	s := strings.TrimPrefix(ref, "oci://")

	// Split tag
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		tag = parts[1]
	} else {
		tag = "latest"
	}
	base := parts[0]

	// Split registry/repo
	i := strings.Index(base, "/")
	if i == -1 {
		// Default to Docker Hub library? Or Error?
		// Let's enforce fully qualified name for now: registry/repo
		return "", "", "", fmt.Errorf("invalid reference, expected registry/repo:tag")
	}

	registry = base[:i]
	repo = base[i+1:]

	// Handle docker.io special case if needed (not implementing full normalization to keep simple)
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	}

	return registry, repo, tag, nil
}
