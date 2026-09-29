package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type s3ListResult struct {
	IsTruncated bool   `xml:"IsTruncated"`
	NextToken   string `xml:"NextContinuationToken"`
	Contents    []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
}

var s3ListHTTPClient = &http.Client{Timeout: 30 * time.Second}
var s3ObjectLockHTTPClient = &http.Client{Timeout: 30 * time.Second}

func listS3(ctx context.Context, target string, credentials Credentials, prefix string) ([]RemoteFile, error) {
	base, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.Trim(base.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("S3 target path must include a bucket")
	}
	bucket := parts[0]
	configuredPrefix := strings.Join(parts[1:], "/")
	objectPrefix := strings.Trim(path.Join(configuredPrefix, prefix), "/")
	queryPrefix := objectPrefix
	if queryPrefix != "" {
		queryPrefix += "/"
	}
	base.Path = "/" + bucket
	base.RawPath = ""
	base.RawQuery = ""
	result := make([]RemoteFile, 0)
	continuation := ""
	for page := 0; page < 1000; page++ {
		query := url.Values{"list-type": []string{"2"}}
		if queryPrefix != "" {
			query.Set("prefix", queryPrefix)
		}
		if continuation != "" {
			query.Set("continuation-token", continuation)
		}
		base.RawQuery = query.Encode()
		base.RawQuery = strings.ReplaceAll(base.RawQuery, "+", "%20")
		request, _, err := signedS3Request(ctx, http.MethodGet, base.String(), credentials, "", "")
		if err != nil {
			return nil, err
		}
		response, err := s3ListHTTPClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("S3 listing failed: %w", err)
		}
		var parsed s3ListResult
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, fmt.Errorf("S3 listing returned HTTP %d", response.StatusCode)
		}
		decodeErr := xml.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&parsed)
		response.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("parse S3 listing: %w", decodeErr)
		}
		for _, item := range parsed.Contents {
			key := strings.TrimPrefix(item.Key, queryPrefix)
			if key == item.Key && queryPrefix != "" {
				continue
			}
			if key == "" || strings.HasSuffix(item.Key, "/") {
				continue
			}
			if err := validateRemoteObject(key); err != nil {
				continue
			}
			modified, _ := time.Parse(time.RFC3339, item.LastModified)
			result = append(result, RemoteFile{Path: key, Size: item.Size, ModTime: modified.UTC()})
		}
		if !parsed.IsTruncated {
			sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
			return result, nil
		}
		if parsed.NextToken == "" {
			return nil, errors.New("S3 listing response omitted its continuation token")
		}
		continuation = parsed.NextToken
	}
	return nil, errors.New("S3 listing exceeded the page limit")
}

func uploadS3(ctx context.Context, target string, credentials Credentials, source, object string, objectLock bool, objectLockDays int) error {
	headers := make(http.Header)
	if objectLock {
		enabled, err := s3BucketObjectLockEnabled(ctx, target, credentials)
		if err != nil {
			return fmt.Errorf("S3 object lock could not be verified: %w", err)
		}
		if !enabled {
			return errors.New("S3 bucket does not have Object Lock enabled")
		}
		headers.Set("x-amz-object-lock-mode", "COMPLIANCE")
		headers.Set("x-amz-object-lock-retain-until-date", time.Now().UTC().AddDate(0, 0, objectLockDays).Format(time.RFC3339))
	}
	request, file, err := signedS3RequestWithHeaders(ctx, http.MethodPut, target, credentials, object, source, headers)
	if err != nil {
		return err
	}
	defer file.Close()
	request.Body = io.NopCloser(file)
	if info, statErr := file.Stat(); statErr == nil {
		request.ContentLength = info.Size()
	}
	response, err := s3ObjectLockHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("S3 upload failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("S3 upload returned HTTP %d", response.StatusCode)
	}
	if objectLock {
		locked, err := s3ObjectRetentionPresent(ctx, target, credentials, object)
		if err != nil {
			return fmt.Errorf("uploaded object retention could not be verified: %w", err)
		}
		if !locked {
			return errors.New("S3 accepted the copy but did not confirm its compliance retention")
		}
	}
	return nil
}

func s3BucketObjectLockEnabled(ctx context.Context, target string, credentials Credentials) (bool, error) {
	endpoint, err := s3BucketEndpoint(target)
	if err != nil {
		return false, err
	}
	endpoint.RawQuery = "object-lock"
	request, _, err := signedS3Request(ctx, http.MethodGet, endpoint.String(), credentials, "", "")
	if err != nil {
		return false, err
	}
	response, err := s3ObjectLockHTTPClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("bucket object-lock check returned HTTP %d", response.StatusCode)
	}
	var value struct {
		Enabled string `xml:"ObjectLockEnabled"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		return false, err
	}
	return value.Enabled == "Enabled", nil
}

func s3ObjectRetentionPresent(ctx context.Context, target string, credentials Credentials, object string) (bool, error) {
	base, err := url.Parse(target)
	if err != nil {
		return false, err
	}
	base.RawQuery = "retention"
	request, _, err := signedS3Request(ctx, http.MethodGet, base.String(), credentials, object, "")
	if err != nil {
		return false, err
	}
	response, err := s3ObjectLockHTTPClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("object retention check returned HTTP %d", response.StatusCode)
	}
	var value struct {
		Mode  string    `xml:"Retention>Mode"`
		Until time.Time `xml:"Retention>RetainUntilDate"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		return false, err
	}
	return value.Mode == "COMPLIANCE" && value.Until.After(time.Now().UTC()), nil
}

func s3BucketEndpoint(target string) (*url.URL, error) {
	base, err := url.Parse(target)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errors.New("invalid S3 target")
	}
	parts := strings.Split(strings.Trim(base.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("S3 target path must include a bucket")
	}
	base.Path = "/" + parts[0]
	base.RawPath = ""
	base.RawQuery = ""
	return base, nil
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
	return signedS3RequestWithHeaders(ctx, method, target, credentials, object, source, nil)
}

func signedS3RequestWithHeaders(ctx context.Context, method, target string, credentials Credentials, object, source string, extra http.Header) (*http.Request, *os.File, error) {
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		return nil, nil, errors.New("S3 credentials require access key and secret key")
	}
	base, err := url.Parse(target)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, nil, errors.New("invalid S3 target")
	}
	if object != "" {
		if err := validateRemoteObject(object); err != nil {
			return nil, nil, err
		}
	}
	if object != "" {
		base.Path = path.Join(base.Path, object)
	}
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
	canonicalHeaderValues := map[string]string{
		"host":                 base.Host,
		"x-amz-content-sha256": hex.EncodeToString(payloadHash[:]),
		"x-amz-date":           amzDate,
	}
	for name, values := range extra {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "x-amz-object-lock-") || len(values) != 1 {
			if file != nil {
				_ = file.Close()
			}
			return nil, nil, errors.New("unsupported S3 request header")
		}
		canonicalHeaderValues[lower] = strings.TrimSpace(values[0])
		request.Header.Set(lower, values[0])
	}
	headerNames := make([]string, 0, len(canonicalHeaderValues))
	for name := range canonicalHeaderValues {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	signedHeaders := strings.Join(headerNames, ";")
	var canonicalHeaders strings.Builder
	for _, name := range headerNames {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(canonicalHeaderValues[name])
		canonicalHeaders.WriteByte('\n')
	}
	canonicalQuery := strings.ReplaceAll(base.Query().Encode(), "+", "%20")
	canonical := method + "\n" + base.EscapedPath() + "\n" + canonicalQuery + "\n" + canonicalHeaders.String() + "\n" + signedHeaders + "\n" + hex.EncodeToString(payloadHash[:])
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
