package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"
	"strings"
)

func URLID(target *ParsedURL, storageIDLen int, storageSalt []byte) string {
	if target == nil {
		return ""
	}

	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme == "" {
		return ""
	}

	hasher := sha256.New()
	hasher.Write(storageSalt)
	hasher.Write([]byte(scheme))

	writeNullOrValue(hasher, target.Password, target.HasPassword)
	writeNullOrValue(hasher, target.User, target.HasUser)
	writeNullOrValue(hasher, target.Host, strings.TrimSpace(target.Host) != "")
	if target.HasPort {
		hasher.Write([]byte(strconv.Itoa(target.Port)))
	} else {
		hasher.Write([]byte{0})
	}

	fullpath := target.Path
	if strings.TrimSpace(fullpath) == "" {
		fullpath = "/"
	}
	fullpath = strings.TrimRight(fullpath, "/")
	hasher.Write([]byte(fullpath))

	if isSecureScheme(scheme) {
		hasher.Write([]byte("s"))
	} else {
		hasher.Write([]byte("i"))
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	if storageIDLen > 0 && storageIDLen < len(sum) {
		return sum[:storageIDLen]
	}
	return sum
}

func writeNullOrValue(hasher hash.Hash, value string, hasValue bool) {
	if !hasValue {
		hasher.Write([]byte{0})
		return
	}
	hasher.Write([]byte(value))
}
