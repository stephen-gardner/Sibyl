package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type (
	vogConn struct {
		*ssh.Client
	}
	gitRepo struct {
		conn vogConn
		path string
	}
)

const gitTimeFormat = "Mon Jan 2 15:04:05 2006 -0700"

func (repo gitRepo) countCommits() (commits int, err error) {
	var out []byte
	cmd := fmt.Sprintf("git -C %s log | grep '^commit' | wc -l", repo.path)
	out, err = repo.conn.runCommand(cmd)
	if err != nil || len(out) == 0 {
		return
	}
	return strconv.Atoi(strings.TrimRight(string(out), "\n"))
}

func (repo gitRepo) getLastUpdate() (lastUpdate time.Time, err error) {
	var out []byte
	cmd := fmt.Sprintf("git -C %s log | grep '^Date:' | head -n1", repo.path)
	out, err = repo.conn.runCommand(cmd)
	if err != nil || len(out) == 0 {
		return
	}
	dateStr := strings.Trim(strings.SplitN(string(out), ":", 2)[1], " \n")
	return time.Parse(gitTimeFormat, dateStr)
}

func (vog vogConn) getGitRepo(repoURL, repoUUID string) gitRepo {
	path := strings.Split(strings.Split(repoURL, ":")[1], "/")
	path[len(path)-1] = repoUUID
	path = append([]string{config.RepoPath}, path...)
	return gitRepo{
		conn: vog,
		path: strings.Join(path, "/"),
	}
}

func (vog *vogConn) runCommand(cmd string) ([]byte, error) {
	session, err := vog.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.Output(cmd)
}

func (vog *vogConn) connect() error {
	key, err := ioutil.ReadFile(config.RepoPrivateKeyPath)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}
	sshConfig := &ssh.ClientConfig{
		User:            config.RepoUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	address := fmt.Sprintf("%s:%d", config.RepoAddress, config.RepoPort)
	conn, err := ssh.Dial("tcp", address, sshConfig)
	vog.Client = conn
	return err
}
