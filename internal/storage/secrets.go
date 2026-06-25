package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// Secret externalization (#42).
//
// Credential values — Basic password, Bearer token, API-key value, OAuth2
// client secret, and Secret-flagged environment variables — must not be written
// into the git-tracked collection YAML. On Save they are split out into a store
// under the OS config dir (outside any repo, so it can never be committed) and
// the collection is written with those fields blanked; on Load they are merged
// back. Keys are positional (mirroring the on-disk seq order), so a save and the
// next load agree. The live in-memory model always keeps the real values, so a
// failed save never loses a credential.

// secretsDirOverride lets tests redirect the store off the real config dir.
var secretsDirOverride string

// secretFile is the on-disk shape of a collection's externalized secrets.
type secretFile struct {
	CollectionDir string            `yaml:"collectionDir"`
	Secrets       map[string]string `yaml:"secrets"`
}

// secretsPath returns the store path for the collection at dir: a per-collection
// file under <config>/helena/secrets/ named by a hash of the absolute dir, so
// distinct collections never collide and the name carries no path info.
func secretsPath(dir string) (string, error) {
	base := secretsDirOverride
	if base == "" {
		base = os.Getenv("HELENA_SECRETS_DIR")
	}
	if base == "" {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(cfg, "helena", "secrets")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(base, hex.EncodeToString(sum[:])+".yml"), nil
}

// writeSecrets persists the secret map for dir (mode 0600). An empty map sweeps
// the store file, so removing the last secret cleans up.
func writeSecrets(dir string, secrets map[string]string) error {
	path, err := secretsPath(dir)
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(secretFile{CollectionDir: dir, Secrets: secrets})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// readSecrets returns the stored secret map for dir, or nil when there is none
// (or it can't be read/parsed — load stays best-effort, falling back to whatever
// the collection YAML carries).
func readSecrets(dir string) map[string]string {
	path, err := secretsPath(dir)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sf secretFile
	if yaml.Unmarshal(data, &sf) != nil {
		return nil
	}
	return sf.Secrets
}

// splitSecrets returns a sanitized deep-enough copy of c with every secret field
// blanked, plus the map of the extracted (non-empty) values. The original c is
// not mutated — the live model keeps its credentials.
func splitSecrets(c model.Collection) (model.Collection, map[string]string) {
	clone := cloneForSecretSplit(c)
	secrets := map[string]string{}
	eachSecret(&clone, func(key string, val *string) {
		if *val != "" {
			secrets[key] = *val
			*val = ""
		}
	})
	return clone, secrets
}

// applySecrets fills any blank secret field in c from the store (by key). A
// field that already has a value — an older, pre-externalization collection —
// is left untouched so it loads unchanged and migrates on the next save.
func applySecrets(c *model.Collection, secrets map[string]string) {
	if len(secrets) == 0 {
		return
	}
	eachSecret(c, func(key string, val *string) {
		if *val == "" {
			if v, ok := secrets[key]; ok {
				*val = v
			}
		}
	})
}

// eachSecret visits every secret-bearing field in c, passing a stable key and a
// pointer to the field (so callers can read or rewrite it). Keys are positional:
// collection root, then root requests, then folders (depth-first), then
// environment variables, then collection-level variables (#80) — matching the
// seq order the files load in.
func eachSecret(c *model.Collection, fn func(key string, val *string)) {
	authSecrets("col", &c.Auth, fn)
	for i := range c.Requests {
		prefix := fmt.Sprintf("r%d", i)
		authSecrets(prefix, &c.Requests[i].Auth, fn)
		variableSecrets(prefix, c.Requests[i].Variables, fn)
	}
	eachFolderSecret("", c.Folders, fn)
	for i := range c.Environments {
		for j := range c.Environments[i].Variables {
			v := &c.Environments[i].Variables[j]
			if v.Secret {
				fn(fmt.Sprintf("e%d/v%d", i, j), &v.Value)
			}
		}
	}
	for j := range c.Variables {
		v := &c.Variables[j]
		if v.Secret {
			fn(fmt.Sprintf("cv%d", j), &v.Value)
		}
	}
}

func eachFolderSecret(prefix string, folders []model.Folder, fn func(string, *string)) {
	for i := range folders {
		p := fmt.Sprintf("%sf%d", prefix, i)
		authSecrets(p, &folders[i].Auth, fn)
		variableSecrets(p, folders[i].Variables, fn)
		for j := range folders[i].Requests {
			rp := fmt.Sprintf("%s/r%d", p, j)
			authSecrets(rp, &folders[i].Requests[j].Auth, fn)
			variableSecrets(rp, folders[i].Requests[j].Variables, fn)
		}
		eachFolderSecret(p+"/", folders[i].Folders, fn)
	}
}

// variableSecrets visits each secret-flagged variable in vars, keyed
// positionally under prefix (e.g. "r0/v1"). Used for request-scoped
// variables (#82); environment and collection variables have their own
// dedicated keying in eachSecret.
func variableSecrets(prefix string, vars []model.Variable, fn func(string, *string)) {
	for j := range vars {
		if vars[j].Secret {
			fn(fmt.Sprintf("%s/v%d", prefix, j), &vars[j].Value)
		}
	}
}

func authSecrets(prefix string, a *model.Auth, fn func(string, *string)) {
	if a.Basic != nil {
		fn(prefix+"/auth/basic.password", &a.Basic.Password)
	}
	if a.Bearer != nil {
		fn(prefix+"/auth/bearer.token", &a.Bearer.Token)
	}
	if a.APIKey != nil {
		fn(prefix+"/auth/apikey.value", &a.APIKey.Value)
	}
	if a.OAuth2 != nil {
		fn(prefix+"/auth/oauth2.clientSecret", &a.OAuth2.ClientSecret)
	}
	if a.WSSE != nil {
		fn(prefix+"/auth/wsse.password", &a.WSSE.Password)
	}
	if a.OAuth1 != nil {
		fn(prefix+"/auth/oauth1.consumerSecret", &a.OAuth1.ConsumerSecret)
		fn(prefix+"/auth/oauth1.tokenSecret", &a.OAuth1.TokenSecret)
	}
	if a.AWSV4 != nil {
		fn(prefix+"/auth/awsv4.secretAccessKey", &a.AWSV4.SecretAccessKey)
		fn(prefix+"/auth/awsv4.sessionToken", &a.AWSV4.SessionToken)
	}
	if a.Digest != nil {
		fn(prefix+"/auth/digest.password", &a.Digest.Password)
	}
}

// cloneForSecretSplit deep-copies exactly the parts splitSecrets mutates — the
// Auth sub-struct pointers and the environment Variable slices — leaving the
// rest shared (it is never written).
func cloneForSecretSplit(c model.Collection) model.Collection {
	c.Auth = cloneAuth(c.Auth)
	c.Folders = cloneFolders(c.Folders)
	c.Requests = cloneRequests(c.Requests)
	c.Environments = cloneEnvs(c.Environments)
	if c.Variables != nil {
		vs := make([]model.Variable, len(c.Variables))
		copy(vs, c.Variables)
		c.Variables = vs
	}
	return c
}

func cloneAuth(a model.Auth) model.Auth {
	if a.Basic != nil {
		b := *a.Basic
		a.Basic = &b
	}
	if a.Bearer != nil {
		b := *a.Bearer
		a.Bearer = &b
	}
	if a.APIKey != nil {
		b := *a.APIKey
		a.APIKey = &b
	}
	if a.OAuth2 != nil {
		b := *a.OAuth2
		a.OAuth2 = &b
	}
	if a.WSSE != nil {
		b := *a.WSSE
		a.WSSE = &b
	}
	if a.OAuth1 != nil {
		b := *a.OAuth1
		a.OAuth1 = &b
	}
	if a.AWSV4 != nil {
		b := *a.AWSV4
		a.AWSV4 = &b
	}
	if a.Digest != nil {
		b := *a.Digest
		a.Digest = &b
	}
	return a
}

func cloneFolders(folders []model.Folder) []model.Folder {
	if folders == nil {
		return nil
	}
	out := make([]model.Folder, len(folders))
	for i, f := range folders {
		f.Auth = cloneAuth(f.Auth)
		f.Folders = cloneFolders(f.Folders)
		f.Requests = cloneRequests(f.Requests)
		if f.Variables != nil {
			vs := make([]model.Variable, len(f.Variables))
			copy(vs, f.Variables)
			f.Variables = vs
		}
		out[i] = f
	}
	return out
}

func cloneRequests(requests []model.Request) []model.Request {
	if requests == nil {
		return nil
	}
	out := make([]model.Request, len(requests))
	for i, r := range requests {
		r.Auth = cloneAuth(r.Auth)
		if r.Variables != nil {
			vs := make([]model.Variable, len(r.Variables))
			copy(vs, r.Variables)
			r.Variables = vs
		}
		out[i] = r
	}
	return out
}

func cloneEnvs(envs []model.Environment) []model.Environment {
	if envs == nil {
		return nil
	}
	out := make([]model.Environment, len(envs))
	for i, e := range envs {
		if e.Variables != nil {
			vs := make([]model.Variable, len(e.Variables))
			copy(vs, e.Variables)
			e.Variables = vs
		}
		out[i] = e
	}
	return out
}
