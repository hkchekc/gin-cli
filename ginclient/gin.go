package ginclient

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"net/http"

	"github.com/G-Node/gin-cli/ginclient/config"
	"github.com/G-Node/gin-cli/ginclient/log"
	"github.com/G-Node/gin-cli/git"
	"github.com/G-Node/gin-cli/git/shell"
	"github.com/G-Node/gin-cli/web"
	gogs "github.com/gogits/go-gogs-client"
)

// High level functions for managing user auth.
// These functions end up performing web calls (using the web package).

// ginerror convenience alias to util.Error
type ginerror = shell.Error

// GINUser represents a API user.
type GINUser struct {
	ID        int64  `json:"id"`
	UserName  string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// New returns a new client for the GIN server.
func New(host string) *Client {
	return &Client{Client: web.New(host)}
}

// AccessToken represents a API access token.
type AccessToken struct {
	Name string `json:"name"`
	Sha1 string `json:"sha1"`
}

// Client is a client interface to the GIN server. Embeds web.Client.
type Client struct {
	*web.Client
	GitAddress string
}

// GetUserKeys fetches the public keys that the user has added to the auth server.
func (gincl *Client) GetUserKeys() ([]gogs.PublicKey, error) {
	fn := "GetUserKeys()"
	var keys []gogs.PublicKey
	res, err := gincl.Get("/api/v1/user/keys")
	if err != nil {
		return nil, err // return error from Get() directly
	}
	switch code := res.StatusCode; {
	case code == http.StatusUnauthorized:
		return nil, ginerror{UError: res.Status, Origin: fn, Description: "authorisation failed"}
	case code == http.StatusInternalServerError:
		return nil, ginerror{UError: res.Status, Origin: fn, Description: "server error"}
	case code != http.StatusOK:
		return nil, ginerror{UError: res.Status, Origin: fn} // Unexpected error
	}

	defer web.CloseRes(res.Body)

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, ginerror{UError: err.Error(), Origin: fn, Description: "failed to read response body"}
	}
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, ginerror{UError: err.Error(), Origin: fn, Description: "failed to parse response body"}
	}
	return keys, nil
}

// RequestAccount requests a specific account by name.
func (gincl *Client) RequestAccount(name string) (gogs.User, error) {
	fn := fmt.Sprintf("RequestAccount(%s)", name)
	var acc gogs.User
	res, err := gincl.Get(fmt.Sprintf("/api/v1/users/%s", name))
	if err != nil {
		return acc, err // return error from Get() directly
	}
	switch code := res.StatusCode; {
	case code == http.StatusNotFound:
		return acc, ginerror{UError: res.Status, Origin: fn, Description: fmt.Sprintf("requested user '%s' does not exist", name)}
	case code == http.StatusUnauthorized:
		return acc, ginerror{UError: res.Status, Origin: fn, Description: "authorisation failed"}
	case code == http.StatusInternalServerError:
		return acc, ginerror{UError: res.Status, Origin: fn, Description: "server error"}
	case code != http.StatusOK:
		return acc, ginerror{UError: res.Status, Origin: fn} // Unexpected error
	}

	defer web.CloseRes(res.Body)

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return acc, ginerror{UError: err.Error(), Origin: fn, Description: "failed to read response body"}
	}
	err = json.Unmarshal(b, &acc)
	if err != nil {
		err = ginerror{UError: err.Error(), Origin: fn, Description: "failed to parse response body"}
	}
	return acc, err
}

// AddKey adds the given key to the current user's authorised keys.
// If force is enabled, any key which matches the new key's description will be overwritten.
func (gincl *Client) AddKey(key, description string, force bool) error {
	fn := "AddKey()"
	newkey := gogs.PublicKey{Key: key, Title: description}

	if force {
		// Attempting to delete potential existing key that matches the title
		_ = gincl.DeletePubKeyByTitle(description)
	}
	res, err := gincl.Post("/api/v1/user/keys", newkey)
	if err != nil {
		return err // return error from Post() directly
	}
	switch code := res.StatusCode; {
	case code == http.StatusUnprocessableEntity:
		return ginerror{UError: res.Status, Origin: fn, Description: "invalid key or key with same name already exists"}
	case code == http.StatusUnauthorized:
		return ginerror{UError: res.Status, Origin: fn, Description: "authorisation failed"}
	case code == http.StatusInternalServerError:
		return ginerror{UError: res.Status, Origin: fn, Description: "server error"}
	case code != http.StatusCreated:
		return ginerror{UError: res.Status, Origin: fn} // Unexpected error
	}
	web.CloseRes(res.Body)
	return nil
}

// DeletePubKey the key with the given ID from the current user's authorised keys.
func (gincl *Client) DeletePubKey(id int64) error {
	fn := "DeletePubKey()"

	address := fmt.Sprintf("/api/v1/user/keys/%d", id)
	res, err := gincl.Delete(address)
	defer web.CloseRes(res.Body)
	if err != nil {
		return err // Return error from Delete() directly
	}
	switch code := res.StatusCode; {
	case code == http.StatusInternalServerError:
		return ginerror{UError: res.Status, Origin: fn, Description: "server error"}
	case code == http.StatusUnauthorized:
		return ginerror{UError: res.Status, Origin: fn, Description: "authorisation failed"}
	case code == http.StatusForbidden:
		return ginerror{UError: res.Status, Origin: fn, Description: "failed to delete key (forbidden)"}
	case code != http.StatusNoContent:
		return ginerror{UError: res.Status, Origin: fn} // Unexpected error
	}
	return nil
}

// DeletePubKeyByTitle removes the key that matches the given title from the current user's authorised keys.
func (gincl *Client) DeletePubKeyByTitle(title string) error {
	log.Write("Searching for key with title '%s'", title)
	keys, err := gincl.GetUserKeys()
	if err != nil {
		log.Write("Error when getting user keys: %v", err)
		return err
	}
	for _, key := range keys {
		if key.Title == title {
			return gincl.DeletePubKey(key.ID)
		}
	}
	return fmt.Errorf("No key with title '%s'", title)
}

// DeletePubKeyByIdx removes the key with the given index from the current user's authorised keys.
// Upon deletion, it returns the title of the key that was deleted.
// Note that the first key has index 1.
func (gincl *Client) DeletePubKeyByIdx(idx int) (string, error) {
	log.Write("Searching for key with index '%d'", idx)
	if idx < 1 {
		log.Write("Invalid index [idx %d]", idx)
		return "", fmt.Errorf("Invalid key index '%d'", idx)
	}
	log.Write("Searching for key with index '%d'", idx)
	keys, err := gincl.GetUserKeys()
	if err != nil {
		log.Write("Error when getting user keys: %v", err)
		return "", err
	}
	if idx > len(keys) {
		log.Write("Invalid index [idx %d > N %d]", idx, len(keys))
		return "", fmt.Errorf("Invalid key index '%d'", idx)
	}
	key := keys[idx-1]
	return key.Title, gincl.DeletePubKey(key.ID)
}

// Login requests a token from the auth server and stores the username and token to file.
// It also generates a key pair for the user for use in git commands.
func (gincl *Client) Login(username, password, clientID string) error {
	fn := "Login()"
	tokenCreate := &gogs.CreateAccessTokenOption{Name: "gin-cli"}
	address := fmt.Sprintf("/api/v1/users/%s/tokens", username)
	res, err := gincl.PostBasicAuth(address, username, password, tokenCreate)
	if err != nil {
		return err // return error from PostBasicAuth directly
	}
	switch code := res.StatusCode; {
	case code == http.StatusInternalServerError:
		return ginerror{UError: res.Status, Origin: fn, Description: "server error"}
	case code == http.StatusUnauthorized:
		return ginerror{UError: res.Status, Origin: fn, Description: "authorisation failed"}
	case code != http.StatusCreated:
		return ginerror{UError: res.Status, Origin: fn} // Unexpected error
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	log.Write("Got response: %s", res.Status)
	token := AccessToken{}
	err = json.Unmarshal(data, &token)
	if err != nil {
		return ginerror{UError: err.Error(), Origin: fn, Description: "failed to parse response body"}
	}
	gincl.Username = username
	gincl.Token = token.Sha1
	log.Write("Login successful. Username: %s", username)

	err = gincl.StoreToken()
	if err != nil {
		return fmt.Errorf("Error while storing token: %s", err.Error())
	}

	MakeHostsFile()

	return gincl.MakeSessionKey()
}

// Logout logs out the currently logged in user in 3 steps:
// 1. Remove the public key matching the current hostname from the server.
// 2. Delete the private key file from the local machine.
// 3. Delete the user token.
func (gincl *Client) Logout() {
	// 1. Delete public key
	hostname, err := os.Hostname()
	if err != nil {
		log.Write("Could not retrieve hostname")
		hostname = unknownhostname
	}

	currentkeyname := fmt.Sprintf("GIN Client: %s@%s", gincl.Username, hostname)
	err = gincl.DeletePubKeyByTitle(currentkeyname)
	if err != nil {
		log.Write(err.Error())
	}

	// 2. Delete private key
	privKeyFile := git.PrivKeyPath(gincl.UserToken.Username)
	err = os.Remove(privKeyFile)
	if err != nil {
		log.Write("Error deleting key file")
	} else {
		log.Write("Private key file deleted")
	}

	err = web.DeleteToken()
	if err != nil {
		log.Write("Error deleting token file")
	}
}

// MakeHostsFile creates a known_hosts file in the config directory based on the server configuration for host key checking.
func MakeHostsFile() {
	conf := config.Read()
	hostkeyfile := git.HostKeyPath()
	ginhostkey := fmt.Sprintln(conf.Servers["gin"].Git.HostKey)
	_ = ioutil.WriteFile(hostkeyfile, []byte(ginhostkey), 0600)
	return
}
