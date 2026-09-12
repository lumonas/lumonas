package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

func uploadS3(ctx context.Context, target string, credentials Credentials, source, object string) error {
	request, file, err := signedS3Request(ctx, http.MethodPut, target, credentials, object, source)
	if err != nil {
		return err
	}
	defer file.Close()
	request.Body = io.NopCloser(file)
	if info, statErr := file.Stat(); statErr == nil {
		request.ContentLength = info.Size()
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("S3 upload failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("S3 upload returned HTTP %d", response.StatusCode)
	}
	return nil
}

func deleteS3(ctx context.Context, target string, credentials Credentials, object string) error {
	request, _, err := signedS3Request(ctx, http.MethodDelete, target, credentials, object, "")
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("S3 delete failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("S3 delete returned HTTP %d", response.StatusCode)
	}
	return nil
}

func downloadS3(ctx context.Context, target string, credentials Credentials, object, destination string) error {
	request, _, err := signedS3Request(ctx, http.MethodGet, target, credentials, object, "")
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("S3 download failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("S3 download returned HTTP %d", response.StatusCode)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, response.Body); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func signedS3Request(ctx context.Context, method, target string, credentials Credentials, object, source string) (*http.Request, *os.File, error) {
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		return nil, nil, errors.New("S3 credentials require access key and secret key")
	}
	base, err := url.Parse(target)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, nil, errors.New("invalid S3 target")
	}
	if err := validateRemoteObject(object); err != nil {
		return nil, nil, err
	}
	base.Path = path.Join(base.Path, object)
	payloadHash := sha256.Sum256(nil)
	var file *os.File
	if source != "" {
		file, err = os.Open(source)
		if err != nil {
			return nil, nil, err
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			file.Close()
			return nil, nil, err
		}
		copy(payloadHash[:], hash.Sum(nil))
		if _, err := file.Seek(0, 0); err != nil {
			file.Close()
			return nil, nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), nil)
	if err != nil {
		if file != nil {
			file.Close()
		}
		return nil, nil, err
	}
	request.Header.Set("Host", base.Host)
	request.Header.Set("x-amz-content-sha256", hex.EncodeToString(payloadHash[:]))
	now := time.Now().UTC()
	amzDate, date := now.Format("20060102T150405Z"), now.Format("20060102")
	request.Header.Set("x-amz-date", amzDate)
	region := strings.TrimSpace(credentials.Region)
	if region == "" {
		region = "us-east-1"
	}
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := method + "\n" + base.EscapedPath() + "\n\n" + "host:" + base.Host + "\n" + "x-amz-content-sha256:" + hex.EncodeToString(payloadHash[:]) + "\n" + "x-amz-date:" + amzDate + "\n\n" + signedHeaders + "\n" + hex.EncodeToString(payloadHash[:])
	canonicalHash := sha256.Sum256([]byte(canonical))
	scope := date + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	key := hmacSHA256([]byte("AWS4"+credentials.SecretKey), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+credentials.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	if credentials.SessionToken != "" {
		request.Header.Set("x-amz-security-token", credentials.SessionToken)
	}
	return request, file, nil
}

func hmacSHA256(key []byte, value string) []byte {
	hash := hmac.New(sha256.New, key)
	_, _ = hash.Write([]byte(value))
	return hash.Sum(nil)
}
