// Package readingfolder describes one complete accepted local page directory.
// It grants no filesystem access and never accepts a client-supplied locator.
package readingfolder

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "path/filepath"
 "sort"
 "strings"
)

const MaxPages = 4096
const MaxPageBytes int64 = 32 << 20
const MaxBytes int64 = 2 << 30

var ErrInventory = errors.New("incomplete or changed image directory")

type Member struct {
 ObjectID string
 RelativePath string
 ObjectIdentity string
 Revision string
 Evidence string
 Size int64
 ModifiedNS int64
 Asset int64
}

type Inventory struct {
 SourceID string
 Incarnation string
 RootIdentity string
 Directory string
 DirectoryIdentity string
 DirectoryRevision string
 Job string
 Members []Member
}

func IsPage(name string) bool {
 switch strings.ToLower(filepath.Ext(name)) {
 case ".jpg", ".jpeg", ".png", ".webp": return true
 default: return false
 }
}

// Seal copies and orders the entire accepted direct page set. Callers must
// already prove the directory observation is complete, current and local.
func Seal(v Inventory) (Inventory, string, error) {
 if v.SourceID == "" || v.Incarnation == "" || v.RootIdentity == "" || v.DirectoryIdentity == "" || v.DirectoryRevision == "" || v.Job == "" || !filepath.IsLocal(v.Directory) || filepath.Clean(v.Directory) != v.Directory || len(v.Members) == 0 || len(v.Members) > MaxPages { return Inventory{}, "", ErrInventory }
 v.Members = append([]Member(nil), v.Members...)
 seen := make(map[string]bool, len(v.Members))
 var total int64
 for _, m := range v.Members {
  if m.ObjectID == "" || m.ObjectIdentity == "" || m.Revision == "" || m.Evidence == "" || m.Asset <= 0 || !filepath.IsLocal(m.RelativePath) || filepath.Clean(m.RelativePath) != m.RelativePath || filepath.Dir(m.RelativePath) != v.Directory || !IsPage(m.RelativePath) || seen[m.RelativePath] || m.Size <= 0 || m.Size > MaxPageBytes || total > MaxBytes-m.Size { return Inventory{}, "", ErrInventory }
  seen[m.RelativePath] = true
  total += m.Size
 }
 sort.Slice(v.Members, func(i, j int) bool { return NaturalLess(filepath.Base(v.Members[i].RelativePath), filepath.Base(v.Members[j].RelativePath)) })
 b, err := json.Marshal(v)
 if err != nil { return Inventory{}, "", err }
 h := sha256.Sum256(b)
 return v, hex.EncodeToString(h[:]), nil
}

// NaturalLess uses bounded decimal runs without integer conversion or overflow.
func NaturalLess(a, b string) bool {
 aa, bb := strings.ToLower(a), strings.ToLower(b)
 for len(aa) > 0 && len(bb) > 0 {
  if aa[0] >= '0' && aa[0] <= '9' && bb[0] >= '0' && bb[0] <= '9' {
   i, j := 0, 0
   for i < len(aa) && aa[i] >= '0' && aa[i] <= '9' { i++ }
   for j < len(bb) && bb[j] >= '0' && bb[j] <= '9' { j++ }
   x, y := strings.TrimLeft(aa[:i], "0"), strings.TrimLeft(bb[:j], "0")
   if len(x) != len(y) { return len(x) < len(y) }
   if x != y { return x < y }
   if i != j { return i < j }
   aa, bb = aa[i:], bb[j:]
   continue
  }
  if aa[0] != bb[0] { return aa[0] < bb[0] }
  aa, bb = aa[1:], bb[1:]
 }
 if aa != bb { return aa < bb }
 return a < b
}
