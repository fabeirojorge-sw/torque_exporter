/*
 * Copyright (C) 2019  Atos Spain SA. All rights reserved.
 *
 * This file is part of torque_exporter.
 *
 * torque_exporter is free software: you can redistribute it and/or modify it 
 * under the terms of the Apache License, Version 2.0 (the License);
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * The software is provided "AS IS", without any warranty of any kind, express 
 * or implied, including but not limited to the warranties of merchantability, 
 * fitness for a particular purpose and noninfringement, in no event shall the 
 * authors or copyright holders be liable for any claim, damages or other 
 * liability, whether in action of contract, tort or otherwise, arising from, 
 * out of or in connection with the software or the use or other dealings in the 
 * software.
 *
 * See DISCLAIMER file for the full disclaimer information and LICENSE and 
 * LICENSE-AGREEMENT files for full license information in the project root.
 *
 * Authors:  Atos Research and Innovation, Atos SPAIN SA
 */

package ssh

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type SSHCommand struct {
	Path string
	Env  []string
}

type SSHConfig struct {
	Config *ssh.ClientConfig
	Host   string
	Port   int
}

type SSHClient struct {
	*ssh.Client
}

type SSHSession struct {
	*ssh.Session
	InBuffer  *bytes.Buffer
	OutBuffer *bytes.Buffer
	ErrBuffer *bytes.Buffer
}

func NewSSHConfigByPassword(user, password, host string, port int) *SSHConfig {
	return &SSHConfig{
		Config: &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
			Timeout:         10 * time.Second,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
		Host: host,
		Port: port,
	}
}

func NewSSHConfigByCertificate(user, key_file, host string, port int) *SSHConfig {
	return &SSHConfig{
		Config: &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				PublicKeyFile(key_file),
			},
			Timeout:         10 * time.Second,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
		Host: host,
		Port: port,
	}
}

func NewSSHConfigByAgent(user, host string, port int) *SSHConfig {
	return &SSHConfig{
		Config: &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				SSHAgent(),
			},
			Timeout:         10 * time.Second,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
		Host: host,
		Port: port,
	}
}

func (config *SSHConfig) NewClient() (*SSHClient, error) {
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", config.Host, 
		config.Port), config.Config)
	if err != nil {
		return nil, err
	}
	return &SSHClient{client}, nil
}

func (client *SSHClient) OpenSession(inBuffer, outBuffer, 
		errBuffer *bytes.Buffer) (*SSHSession, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		session.Close()
		return nil, err
	}

	// Setup the buffers
	ses := &SSHSession{Session: session, InBuffer: inBuffer, 
		OutBuffer: outBuffer, ErrBuffer: errBuffer}
	if err := ses.setupSessionBuffers(); err != nil {
		return nil, err
	}
	return ses, nil
}

func (session *SSHSession) setupSessionBuffers() error {
	if session.InBuffer != nil {
		stdin, err := session.StdinPipe()
		if err != nil {
			return err
		}
		go io.Copy(stdin, session.InBuffer)
	}

	if session.OutBuffer != nil {
		stdout, err := session.StdoutPipe()
		if err != nil {
			return err
		}
		go io.Copy(session.OutBuffer, stdout)
	}

	if session.ErrBuffer != nil {
		stderr, err := session.StderrPipe()
		if err != nil {
			return err
		}
		go io.Copy(session.ErrBuffer, stderr)
	}

	return nil
}

func (session *SSHSession) RunCommand(cmd *SSHCommand) error {
	if err := session.setupCommand(cmd); err != nil {
		return err
	}

	err := session.Run(cmd.Path)
	return err
}

func (session *SSHSession) setupCommand(cmd *SSHCommand) error {
	// TODO(emepetres) clear env before setting a new one?
	for _, env := range cmd.Env {
		variable := strings.Split(env, "=")
		if len(variable) != 2 {
			continue
		}

		if err := session.Setenv(variable[0], variable[1]); err != nil {
			return err
		}
	}

	return nil
}

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func SSHAgent() ssh.AuthMethod {
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); 
		err == nil {
		return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
	}
	return nil
}
