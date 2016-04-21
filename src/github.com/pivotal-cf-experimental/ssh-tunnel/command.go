package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ssh"

	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

type Command struct {
	BindIP   IPFlag `long:"bind-ip"         default:"0.0.0.0" description:"IP address on which to listen for SSH."`
	BindPort uint16 `long:"bind-port"       default:"2222"    description:"Port on which to listen for SSH."`
	PeerIP   string `long:"peer-ip"         required:"true"   description:"IP address for this tunnel host."`

	// AuthorizedKeysPath FileFlag `long:"authorized-keys" required:"true"   description:"Path to file containing keys to authorize, in SSH authorized_keys format."`
	ServerKeyPath FileFlag `long:"server-key"      required:"true"   description:"Path to the private key to use for the SSH tunnel."`
	// SessionKeyPath FileFlag `long:"session-key"     required:"true"   description:"Path to private key to use when signing tokens for registration."`
	logger lager.Logger
}

func (cmd *Command) Execute(args []string) error {
	runner, err := cmd.Runner(args)
	if err != nil {
		return err
	}

	return <-ifrit.Invoke(sigmon.New(runner)).Wait()
}

func (cmd *Command) Runner(args []string) (ifrit.Runner, error) {
	cmd.logger = lager.NewLogger("ssh-tunnel")
	cmd.logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.DEBUG))

	// authorizedKeys, err := cmd.loadAuthorizedKeys()
	// if err != nil {
	//   return nil, fmt.Errorf("Failed to load authorized keys: %s", err)
	// }

	// sessionKey, err := cmd.loadSessionKey()
	// if err != nil {
	// 	return nil, fmt.Errorf("Failed to load session signing key: %s", err)
	// }

	// sessionId -> authorizedToken
	sessionTokens := make(map[string]string)

	config, err := cmd.configureServer(sessionTokens)
	if err != nil {
		return nil, fmt.Errorf("Failed to configure SSH server: %s", err)
	}

	address := fmt.Sprintf("%s:%d", cmd.BindIP, cmd.BindPort)

	server := &tunnelServer{
		logger:        cmd.logger,
		config:        config,
		tunnelHost:    cmd.PeerIP,
		sessionTokens: sessionTokens,
	}

	return tunnelRunner{cmd.logger, server, address}, nil
}

// func (cmd *Command) configureServer(authorizedKeys []ssh.PublicKey)
func (cmd *Command) configureServer(sessionTokens map[string]string) (*ssh.ServerConfig, error) {
	// certChecker := &ssh.CertChecker{}

	config := &ssh.ServerConfig{
		// PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		// 	user := conn.User()
		//
		// 	if user == "remote" {
		// 		token := string(pass)
		// 		matched := false
		//
		// 		for _, authorizedToken := range sessionTokens {
		// 			cmd.logger.Info(fmt.Sprintf("Checking token: %s", authorizedToken))
		// 			if authorizedToken == token {
		// 				matched = true
		// 				break
		// 			}
		// 		}
		//
		// 		if matched {
		// 			cmd.logger.Info(fmt.Sprintf("User logged in: %s", user))
		// 			return nil, nil
		// 		}
		// 	}
		//
		// 	return nil, fmt.Errorf("session rejected for %s", user)
		// },

		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			user := conn.User()
			if user == "server" {
				// TODO: check actual AuthorizedKeys
				token, err := GenerateToken()
				if err != nil {
					return nil, err
				}

				cmd.logger.Info(fmt.Sprintf("Added token: %s", token))
				sessionTokens[string(conn.SessionID())] = token
				return nil, nil
			}

			token := user
			matched := false

			for _, authorizedToken := range sessionTokens {
				cmd.logger.Info(fmt.Sprintf("Checking token: %s", authorizedToken))
				if authorizedToken == token {
					matched = true
					break
				}
			}

			if matched {
				cmd.logger.Info(fmt.Sprintf("User logged in: %s", user))
				return nil, nil
			}

			return nil, errors.New("Bad Key or Token")
		},
	}

	privateBytes, err := ioutil.ReadFile(string(cmd.ServerKeyPath))
	if err != nil {
		return nil, err
	}

	privateKey, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, err
	}

	config.AddHostKey(privateKey)

	return config, nil
}

// func (cmd *Command) loadAuthorizedKeys() ([]ssh.PublicKey, error) {
//   authorizedKeysBytes, err := ioutil.ReadFile(string(cmd.AuthorizedKeysPath))
//   if err != nil {
//     return nil, err
//   }
//
//   var authorizedKeys [].ssh.PublicKey
//
//   for {
//     key, _, _, rest, err := ssh.ParseAuthorizedKey(authorizedKeysBytes)
//     if err != nil {
//       break
//     }
//
//     authorizedKeys = append(authorizedKeys, key)
//     authorizedKeysBytes = rest
//   }
//
//   return authorizedKeys, nil
// }

// func (cmd *Command) loadSessionKey() (*rsa.PrivateKey, error) {
// 	rsaKeyBlob, err := ioutil.ReadFile(string(cmd.SessionKeyPath))
// 	if err != nil {
// 		return nil, fmt.Errorf("Failed to read session signing key file: %s", err)
// 	}
//
// 	sessionKey, err := jwt.ParseRSAPrivateKeyFromPEM(rsaKeyBlob)
// 	if err != nil {
// 		return nil, fmt.Errorf("Failed to parse session signing key as RSA: %s", err)
// 	}
//
// 	return sessionKey, nil
// }
