// Copyright 2023 Ant Group Co., Ltd.
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

package assist

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteOneFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-write-one-file")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name         string
		header       *tar.Header
		content      string
		adjustLink   bool
		skipUnNormal bool
		expectedErr  bool
		setupFunc    func(t *testing.T, targetWd string)
		validateFunc func(t *testing.T, targetPath string)
	}{
		{
			name: "directory type",
			header: &tar.Header{
				Name:     "testdir",
				Typeflag: tar.TypeDir,
				Mode:     0755,
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, targetPath string) {
				info, err := os.Stat(targetPath)
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			},
		},
		{
			name: "regular file type",
			header: &tar.Header{
				Name:     "testfile.txt",
				Typeflag: tar.TypeReg,
				Mode:     0644,
				Size:     11,
			},
			content:     "hello world",
			expectedErr: false,
			validateFunc: func(t *testing.T, targetPath string) {
				content, err := os.ReadFile(targetPath)
				assert.NoError(t, err)
				assert.Equal(t, "hello world", string(content))
			},
		},
		{
			name: "regular file type with existing file",
			header: &tar.Header{
				Name:     "existing.txt",
				Typeflag: tar.TypeReg,
				Mode:     0644,
				Size:     11,
			},
			content:     "new content",
			expectedErr: false,
			setupFunc: func(t *testing.T, targetWd string) {
				existingPath := filepath.Join(targetWd, "existing.txt")
				os.WriteFile(existingPath, []byte("old content"), 0644)
			},
			validateFunc: func(t *testing.T, targetPath string) {
				content, err := os.ReadFile(targetPath)
				assert.NoError(t, err)
				assert.Equal(t, "new content", string(content))
			},
		},
		{
			name: "hard link type - relative path",
			header: &tar.Header{
				Name:     "hardlink.txt",
				Typeflag: tar.TypeLink,
				Linkname: "testfile.txt",
				Mode:     0644,
			},
			expectedErr: false,
			setupFunc: func(t *testing.T, targetWd string) {
				sourcePath := filepath.Join(targetWd, "testfile.txt")
				os.WriteFile(sourcePath, []byte("source content"), 0644)
			},
			validateFunc: func(t *testing.T, targetPath string) {
				info, err := os.Lstat(targetPath)
				assert.NoError(t, err)
				assert.False(t, info.Mode()&os.ModeSymlink != 0)
				assert.False(t, info.IsDir())
			},
		},
		{
			name: "hard link type - absolute path with adjustLink=true",
			header: &tar.Header{
				Name:     "hardlink_abs.txt",
				Typeflag: tar.TypeLink,
				Linkname: "/testfile.txt",
				Mode:     0644,
			},
			adjustLink:  true,
			expectedErr: false,
			setupFunc: func(t *testing.T, targetWd string) {
				sourcePath := filepath.Join(targetWd, "testfile.txt")
				os.WriteFile(sourcePath, []byte("source content"), 0644)
			},
			validateFunc: func(t *testing.T, targetPath string) {
				info, err := os.Lstat(targetPath)
				assert.NoError(t, err)
				assert.False(t, info.Mode()&os.ModeSymlink != 0)
			},
		},
		{
			name: "hard link type - invalid path escapes directory",
			header: &tar.Header{
				Name:     "hardlink_invalid.txt",
				Typeflag: tar.TypeLink,
				Linkname: "../../../etc/passwd",
				Mode:     0644,
			},
			expectedErr: true,
		},
		{
			name: "symbolic link type - relative path",
			header: &tar.Header{
				Name:     "symlink.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "testfile.txt",
				Mode:     0644,
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, targetPath string) {
				info, err := os.Lstat(targetPath)
				assert.NoError(t, err)
				assert.True(t, info.Mode()&os.ModeSymlink != 0)

				linkTarget, err := os.Readlink(targetPath)
				assert.NoError(t, err)
				assert.Equal(t, "testfile.txt", linkTarget)
			},
		},
		{
			name: "symbolic link type - absolute path with adjustLink=true",
			header: &tar.Header{
				Name:     "symlink_abs.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "/testfile.txt",
				Mode:     0644,
			},
			adjustLink:  true,
			expectedErr: false,
			validateFunc: func(t *testing.T, targetPath string) {
				info, err := os.Lstat(targetPath)
				assert.NoError(t, err)
				assert.True(t, info.Mode()&os.ModeSymlink != 0)
			},
		},
		{
			name: "symbolic link type - absolute path with adjustLink=false",
			header: &tar.Header{
				Name:     "symlink_abs_no_adjust.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "/testfile.txt",
				Mode:     0644,
			},
			adjustLink:  false,
			expectedErr: false,
			validateFunc: func(t *testing.T, targetPath string) {
				linkTarget, err := os.Readlink(targetPath)
				assert.NoError(t, err)
				assert.Equal(t, "/testfile.txt", linkTarget)
			},
		},
		{
			name: "symbolic link type - invalid path escapes directory",
			header: &tar.Header{
				Name:     "symlink_invalid.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "../../../etc/passwd",
				Mode:     0644,
			},
			adjustLink:  true,
			expectedErr: false,
		},
		{
			name: "unsupported file type with skipUnNormalFile=false",
			header: &tar.Header{
				Name:     "unsupported",
				Typeflag: tar.TypeFifo,
				Mode:     0644,
			},
			skipUnNormal: false,
			expectedErr:  true,
		},
		{
			name: "unsupported file type with skipUnNormalFile=true",
			header: &tar.Header{
				Name:     "unsupported",
				Typeflag: tar.TypeFifo,
				Mode:     0644,
			},
			skipUnNormal: true,
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(tempDir, "test-*")
			assert.NoError(t, err)

			if tt.setupFunc != nil {
				tt.setupFunc(t, testDir)
			}

			targetPath := filepath.Join(testDir, tt.header.Name)

			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)

			err = tw.WriteHeader(tt.header)
			assert.NoError(t, err)

			if tt.header.Typeflag == tar.TypeReg {
				content := tt.content
				if content == "" {
					content = "test content"
				}
				_, err = tw.Write([]byte(content))
				assert.NoError(t, err)
			}

			tw.Close()

			tr := tar.NewReader(&buf)

			header, err := tr.Next()
			assert.NoError(t, err)

			err = writeOneFile(header, tr, testDir, targetPath, tt.adjustLink, tt.skipUnNormal)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, targetPath)
				}
			}
		})
	}
}

func TestWriteOneFile_FileOpenRetrySuccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-retry")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "testfile.txt")
	os.Mkdir(filePath, 0755)

	header := &tar.Header{
		Name:     "testfile.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     4,
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = tw.WriteHeader(header)
	assert.NoError(t, err)
	_, err = tw.Write([]byte("test"))
	assert.NoError(t, err)
	tw.Close()

	tr := tar.NewReader(&buf)
	header, err = tr.Next()
	assert.NoError(t, err)

	err = writeOneFile(header, tr, tempDir, filePath, false, false)
	assert.NoError(t, err)

	info, err := os.Stat(filePath)
	assert.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
}

func TestWriteOneFile_IOCopyError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-iocopy-error")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "testfile.txt")

	header := &tar.Header{
		Name:     "testfile.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     100,
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = tw.WriteHeader(header)
	assert.NoError(t, err)
	_, err = tw.Write([]byte("short"))
	assert.NoError(t, err)
	tw.Close()

	tr := tar.NewReader(&buf)
	header, err = tr.Next()
	assert.NoError(t, err)

	err = writeOneFile(header, tr, tempDir, filePath, false, false)
	assert.Error(t, err)
}

func TestWriteOneFile_HardLinkAbsolutePathError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-hardlink-abs-error")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	header := &tar.Header{
		Name:     "hardlink.txt",
		Typeflag: tar.TypeLink,
		Linkname: "/nonexistent/path",
		Mode:     0644,
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = tw.WriteHeader(header)
	assert.NoError(t, err)
	tw.Close()

	tr := tar.NewReader(&buf)
	header, err = tr.Next()
	assert.NoError(t, err)

	targetPath := filepath.Join(tempDir, "hardlink.txt")
	err = writeOneFile(header, tr, tempDir, targetPath, true, false)
	assert.Error(t, err)
}

func TestWriteOneFile_AbsolutePathError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-abs-error")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	sourceFile := filepath.Join(tempDir, "source.txt")
	os.WriteFile(sourceFile, []byte("content"), 0644)

	header := &tar.Header{
		Name:     "hardlink.txt",
		Typeflag: tar.TypeLink,
		Linkname: "../../../etc/passwd",
		Mode:     0644,
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = tw.WriteHeader(header)
	assert.NoError(t, err)
	tw.Close()

	tr := tar.NewReader(&buf)
	header, err = tr.Next()
	assert.NoError(t, err)

	targetPath := filepath.Join(tempDir, "hardlink.txt")
	err = writeOneFile(header, tr, tempDir, targetPath, false, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")
}

func TestWriteOneFile_HardLinkRelativePathError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-hardlink-rel-error")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	header := &tar.Header{
		Name:     "hardlink.txt",
		Typeflag: tar.TypeLink,
		Linkname: "../../../etc/passwd",
		Mode:     0644,
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = tw.WriteHeader(header)
	assert.NoError(t, err)
	tw.Close()

	tr := tar.NewReader(&buf)
	header, err = tr.Next()
	assert.NoError(t, err)

	targetPath := filepath.Join(tempDir, "hardlink.txt")
	err = writeOneFile(header, tr, tempDir, targetPath, false, false)
	assert.Error(t, err)
}
