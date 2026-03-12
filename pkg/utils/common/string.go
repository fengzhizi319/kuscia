// Copyright 2024 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"io"
	"math/rand"
	"strings"
)

var letters = []byte("23456789abcdefghijklmnpqrstuvwxyzABCDEFGHIJKLMNPQRSTUVWXYZ")

func GenerateRandomBytes(l int) []byte {
	b := make([]byte, l)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

// GenerateID generates a random unique id.
func GenerateID(len int) string {
	b := make([]byte, len)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CompressString compress string
func CompressString(input string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zlib.NewWriter(&buffer)
	_, err := writer.Write([]byte(input))
	if err != nil {
		return nil, err
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecompressString decompress string
func DecompressString(input []byte) (string, error) {
	reader, err := zlib.NewReader(bytes.NewReader(input))
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// Limit the maximum decompressed size to prevent decompression bomb attacks
	// 10MB limit should be sufficient for most legitimate use cases
	maxDecompressedSize := int64(10 * 1024 * 1024) // 10MB
	limitedReader := io.LimitReader(reader, maxDecompressedSize)

	var buffer bytes.Buffer
	bytesCopied, err := io.Copy(&buffer, limitedReader)
	if err != nil {
		return "", err
	}

	// Check if we hit the limit by comparing bytes copied with the limit
	// If we copied exactly the limit amount, the original data was likely larger
	if bytesCopied >= maxDecompressedSize {
		return "", io.ErrShortBuffer
	}

	return buffer.String(), nil
}

// SliceToAnnotationString split annotation from string to slice  by ","
func SliceToAnnotationString(slice []string) (AnnotationValue string) {
	for _, v := range slice {
		if AnnotationValue == "" {
			AnnotationValue = v
			continue
		}
		AnnotationValue += "," + v
	}
	return
}

// AnnotationStringToSlice split value by "_"
func AnnotationStringToSlice(AnnotationValue string) []string {
	return strings.Split(AnnotationValue, ",")
}
