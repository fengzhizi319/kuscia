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

package starter

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRawStarter(t *testing.T) {
	c := createTestInitConfig(t)
	s, err := NewRawStarter(c)
	assert.NoError(t, err)
	assert.NoError(t, s.Start())
	assert.NoError(t, s.Wait())
	assert.Equal(t, 0, s.Command().ProcessState.ExitCode())
	assert.NoError(t, s.Release())
}

func TestValidateCmdLine(t *testing.T) {
	tests := []struct {
		name     string
		cmdLine  []string
		expected []string
		wantErr  error
	}{
		{
			name:     "normal command",
			cmdLine:  []string{"/bin/echo", "hello", "world"},
			expected: []string{"/bin/echo", "hello", "world"},
			wantErr:  nil,
		},
		{
			name:     "command with injection - command",
			cmdLine:  []string{"/bin/echo; rm -rf /", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "arg with injection",
			cmdLine:  []string{"/bin/echo", "hello; rm -rf /"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "command with pipe",
			cmdLine:  []string{"/bin/echo|cat", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "arg with pipe",
			cmdLine:  []string{"/bin/echo", "hello|cat"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "command with backticks",
			cmdLine:  []string{"/bin/echo`whoami`", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "arg with backticks",
			cmdLine:  []string{"/bin/echo", "hello`whoami`"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "relative path command",
			cmdLine:  []string{"./evil", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "empty command line",
			cmdLine:  []string{},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "command with parentheses",
			cmdLine:  []string{"/bin/echo(x)", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "arg with parentheses",
			cmdLine:  []string{"/bin/echo", "test(x)"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "command with shell variable",
			cmdLine:  []string{"/bin/echo $HOME", "test"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
		{
			name:     "arg with shell variable",
			cmdLine:  []string{"/bin/echo", "test $HOME"},
			expected: nil,
			wantErr:  syscall.EINVAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateCmdLine(tt.cmdLine)
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
