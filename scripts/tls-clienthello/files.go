package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

type outputFile struct {
	path       string
	data       []byte
	parentPath string
	parentInfo os.FileInfo
}

type outputAnchor struct {
	output outputFile
	root   *os.Root
	name   string
}

type stagedOutput struct {
	name string
	info os.FileInfo
}

func readSpec(path string) ([]byte, error) {
	if !validLocalPath(path) {
		return nil, fmt.Errorf("--spec must be a valid local regular-file path")
	}
	file, err := openRegularRead(path)
	if err != nil {
		return nil, fmt.Errorf("--spec must be an accessible regular file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, tlsprofile.MaxSpecBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read --spec")
	}
	if len(data) > tlsprofile.MaxSpecBytes {
		return nil, fmt.Errorf("--spec exceeds the 65536-byte limit")
	}
	return data, nil
}

func prepareOutputs(profilePath, metadataPath string, force bool) ([]outputFile, error) {
	paths := []string{profilePath, metadataPath}
	outputs := make([]outputFile, 0, len(paths))
	for _, path := range paths {
		if !validLocalPath(path) {
			return nil, fmt.Errorf("outputs require valid local regular-file paths")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve an output path")
		}
		parentPath := filepath.Dir(absolute)
		parent, err := filepath.EvalSymlinks(parentPath)
		if err != nil {
			return nil, fmt.Errorf("output directories must already exist")
		}
		absolute = filepath.Join(parent, filepath.Base(absolute))
		outputs = append(outputs, outputFile{path: absolute, parentPath: parentPath})
	}
	anchors, err := openOutputAnchors(outputs)
	if err != nil {
		return nil, err
	}
	defer closeOutputAnchors(anchors)
	if err := validateOutputAnchors(anchors, force); err != nil {
		return nil, err
	}
	for index, anchor := range anchors {
		outputs[index] = anchor.output
	}
	return outputs, nil
}

func openOutputAnchors(outputs []outputFile) ([]outputAnchor, error) {
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return nil, fmt.Errorf("anchored output publication is unsupported on this platform")
	}
	anchors := make([]outputAnchor, 0, len(outputs))
	complete := false
	defer func() {
		if !complete {
			closeOutputAnchors(anchors)
		}
	}()
	for _, output := range outputs {
		root, err := os.OpenRoot(filepath.Dir(output.path))
		if err != nil {
			return nil, fmt.Errorf("cannot anchor a validated output directory")
		}
		anchors = append(anchors, outputAnchor{output: output, root: root, name: filepath.Base(output.path)})
		info, err := root.Stat(".")
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("output parent must be a directory")
		}
		if output.parentInfo != nil && !os.SameFile(output.parentInfo, info) {
			return nil, fmt.Errorf("output parent changed since validation")
		}
		anchors[len(anchors)-1].output.parentInfo = info
	}
	complete = true
	return anchors, nil
}

func closeOutputAnchors(anchors []outputAnchor) {
	for _, anchor := range anchors {
		anchor.root.Close()
	}
}

func validateOutputParents(anchors []outputAnchor) error {
	for _, anchor := range anchors {
		for _, parent := range []string{anchor.output.parentPath, filepath.Dir(anchor.output.path)} {
			info, err := os.Stat(parent)
			if err != nil || !info.IsDir() || !os.SameFile(anchor.output.parentInfo, info) {
				return fmt.Errorf("output parent changed since validation")
			}
		}
	}
	return nil
}

func validateOutputAnchors(anchors []outputAnchor, force bool) error {
	if len(anchors) != 2 {
		return fmt.Errorf("publication requires separate profile and metadata outputs")
	}
	if err := validateOutputParents(anchors); err != nil {
		return err
	}
	infos := make([]os.FileInfo, 0, len(anchors))
	for _, anchor := range anchors {
		info, err := checkOutput(anchor.root, anchor.name, force)
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}
	sameName := anchors[0].name == anchors[1].name
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		sameName = strings.EqualFold(anchors[0].name, anchors[1].name)
	}
	if os.SameFile(anchors[0].output.parentInfo, anchors[1].output.parentInfo) && sameName ||
		infos[0] != nil && infos[1] != nil && os.SameFile(infos[0], infos[1]) {
		return fmt.Errorf("profile and metadata output paths must not collide")
	}
	return nil
}

func checkOutput(root *os.Root, name string, force bool) (os.FileInfo, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot inspect an output destination")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("output destinations must be regular files, not symlinks, directories or special files")
	}
	if !force {
		return nil, fmt.Errorf("output already exists; use --force to replace regular files explicitly")
	}
	return info, nil
}

func publishOutputs(outputs []outputFile, force bool) error {
	for _, output := range outputs {
		if output.parentInfo == nil || output.parentPath == "" {
			return fmt.Errorf("output destinations must be prepared before publication")
		}
	}
	anchors, err := openOutputAnchors(outputs)
	if err != nil {
		return err
	}
	defer closeOutputAnchors(anchors)
	if err := validateOutputAnchors(anchors, force); err != nil {
		return err
	}
	return publishOutputAnchors(anchors, force, publishAnchoredOutput)
}

func publishOutputAnchors(anchors []outputAnchor, force bool, publish func(outputAnchor, stagedOutput, bool) error) error {
	temporary := make([]stagedOutput, 0, len(anchors))
	defer func() {
		for index, staged := range temporary {
			removeStagedOutput(anchors[index].root, staged)
		}
	}()
	for _, anchor := range anchors {
		staged, err := stageOutput(anchor)
		if err != nil {
			return err
		}
		temporary = append(temporary, staged)
	}
	for index, anchor := range anchors {
		if err := validateOutputAnchors(anchors, true); err != nil {
			if !force {
				rollbackPublished(anchors[:index], temporary[:index])
			}
			return err
		}
		if err := publish(anchor, temporary[index], force); err != nil {
			if !force {
				rollbackPublished(anchors[:index], temporary[:index])
			}
			return err
		}
		if err := validateOutputParents(anchors); err != nil {
			if !force {
				rollbackPublished(anchors[:index+1], temporary[:index+1])
			}
			return err
		}
	}
	return nil
}

func stageOutput(anchor outputAnchor) (stagedOutput, error) {
	var file *os.File
	var name string
	for attempt := 0; attempt < 10; attempt++ {
		name = ".clienthello-" + rand.Text()
		var err error
		file, err = anchor.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return stagedOutput{}, fmt.Errorf("cannot stage a secure output file")
		}
	}
	if file == nil {
		return stagedOutput{}, fmt.Errorf("cannot stage a unique secure output file")
	}
	info, statErr := file.Stat()
	staged := stagedOutput{name: name, info: info}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			if staged.info == nil {
				anchor.root.Remove(staged.name)
			} else {
				removeStagedOutput(anchor.root, staged)
			}
		}
	}()
	if statErr != nil || !info.Mode().IsRegular() {
		return stagedOutput{}, fmt.Errorf("cannot verify a secure output file")
	}
	if err := file.Chmod(0600); err != nil {
		return stagedOutput{}, fmt.Errorf("cannot secure an output file")
	}
	if _, err := file.Write(anchor.output.data); err != nil {
		return stagedOutput{}, fmt.Errorf("cannot write a complete secure output file")
	}
	if err := file.Sync(); err != nil {
		return stagedOutput{}, fmt.Errorf("cannot synchronize an output file")
	}
	if err := file.Close(); err != nil {
		return stagedOutput{}, fmt.Errorf("cannot close an output file")
	}
	complete = true
	return staged, nil
}

func publishAnchoredOutput(anchor outputAnchor, staged stagedOutput, force bool) error {
	if _, err := checkOutput(anchor.root, anchor.name, force); err != nil {
		return err
	}
	info, err := anchor.root.Lstat(staged.name)
	if err != nil || !info.Mode().IsRegular() || staged.info == nil || !os.SameFile(info, staged.info) {
		return fmt.Errorf("staged output changed before publication")
	}
	if force {
		err = anchor.root.Rename(staged.name, anchor.name)
	} else {
		err = anchor.root.Link(staged.name, anchor.name)
	}
	if err != nil {
		return fmt.Errorf("cannot publish output without changing its overwrite policy")
	}
	return nil
}

func removeStagedOutput(root *os.Root, staged stagedOutput) {
	info, err := root.Lstat(staged.name)
	if err == nil && staged.info != nil && os.SameFile(info, staged.info) {
		root.Remove(staged.name)
	}
}

func rollbackPublished(anchors []outputAnchor, temporary []stagedOutput) {
	for index, anchor := range anchors {
		published, err := anchor.root.Lstat(anchor.name)
		if err == nil && temporary[index].info != nil && os.SameFile(published, temporary[index].info) {
			anchor.root.Remove(anchor.name)
		}
	}
}
