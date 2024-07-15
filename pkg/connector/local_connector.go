/*
Copyright 2023 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package connector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
)

var _ Connector = &localConnector{}
var _ GatherFacts = &localConnector{}

type localConnector struct {
	Cmd exec.Interface
}

func (c *localConnector) Init(ctx context.Context) error {
	return nil
}

func (c *localConnector) Close(ctx context.Context) error {
	return nil
}

// PutFile copy src file to dst file. src is the local filename, dst is the local filename.
func (c *localConnector) PutFile(ctx context.Context, src []byte, dst string, mode fs.FileMode) error {
	if _, err := os.Stat(filepath.Dir(dst)); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dst), mode); err != nil {
			klog.V(4).ErrorS(err, "Failed to create local dir", "dst_file", dst)
			return err
		}
	}
	return os.WriteFile(dst, src, mode)
}

// FetchFile copy src file to dst writer. src is the local filename, dst is the local writer.
func (c *localConnector) FetchFile(ctx context.Context, src string, dst io.Writer) error {
	var err error
	file, err := os.Open(src)
	if err != nil {
		klog.V(4).ErrorS(err, "Failed to read local file failed", "src_file", src)
		return err
	}
	if _, err := io.Copy(dst, file); err != nil {
		klog.V(4).ErrorS(err, "Failed to copy local file", "src_file", src)
		return err
	}
	return nil
}

func (c *localConnector) ExecuteCommand(ctx context.Context, cmd string) ([]byte, error) {
	klog.V(4).InfoS("exec local command", "cmd", cmd)
	return c.Cmd.CommandContext(ctx, "/bin/sh", "-c", cmd).CombinedOutput()
}

func (c *localConnector) Info(ctx context.Context) (map[string]any, error) {
	switch runtime.GOOS {
	case "linux":
		// os information
		osVars := make(map[string]any)
		var osRelease bytes.Buffer
		if err := c.FetchFile(ctx, "/etc/os-release", &osRelease); err != nil {
			return nil, fmt.Errorf("failed to fetch os-release: %w", err)
		}
		osVars["release"] = convertBytesToMap(osRelease.Bytes(), "=")
		kernel, err := c.ExecuteCommand(ctx, "uname -r")
		if err != nil {
			return nil, fmt.Errorf("get kernel version error: %w", err)
		}
		osVars["kernel_version"] = string(bytes.TrimSuffix(kernel, []byte("\n")))
		hn, err := c.ExecuteCommand(ctx, "hostname")
		if err != nil {
			return nil, fmt.Errorf("get hostname error: %w", err)
		}
		osVars["hostname"] = string(bytes.TrimSuffix(hn, []byte("\n")))
		arch, err := c.ExecuteCommand(ctx, "arch")
		if err != nil {
			return nil, fmt.Errorf("get arch error: %w", err)
		}
		osVars["architecture"] = string(bytes.TrimSuffix(arch, []byte("\n")))

		// process information
		procVars := make(map[string]any)
		var cpu bytes.Buffer
		if err := c.FetchFile(ctx, "/proc/cpuinfo", &cpu); err != nil {
			return nil, fmt.Errorf("get cpuinfo error: %w", err)
		}
		procVars["cpuInfo"] = convertBytesToSlice(cpu.Bytes(), ":")
		var mem bytes.Buffer
		if err := c.FetchFile(ctx, "/proc/meminfo", &mem); err != nil {
			return nil, fmt.Errorf("get meminfo error: %w", err)
		}
		procVars["memInfo"] = convertBytesToMap(mem.Bytes(), ":")

		return map[string]any{
			"os":      osVars,
			"process": procVars,
		}, nil
	default:
		klog.V(4).ErrorS(nil, "Unsupported platform", "platform", runtime.GOOS)
		return nil, nil
	}
}