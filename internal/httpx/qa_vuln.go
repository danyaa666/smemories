package httpx

import "golang.org/x/crypto/ssh"

// QAVuln calls code with a known vulnerability in this old x/crypto.
func QAVuln(c ssh.Conn, cfg *ssh.ServerConfig) {
	_, _, _, _ = ssh.NewServerConn(nil, cfg)
	_, _, _, _, _ = ssh.ParseAuthorizedKey(nil)
}
