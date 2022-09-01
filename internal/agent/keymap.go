package agent

import (
	"bytes"
	"sort"
)

type DB struct {
	alloc  func() interface{}
	values chan interface{}
}

func newDB(size int) *DB {
	return &DB{
		values: make(chan interface{}, size),
	}
}

func (db *DB) init(alloc func() interface{}) {
	db.alloc = alloc
	for i := 0; i < cap(db.values); i++ {
		db.values <- db.alloc()
	}
}

func (db *DB) put(obj interface{}) {
	select {
	case db.values <- obj:
	default:
	}
}

func (db *DB) get() interface{} {
	var v interface{}
	select {
	case v = <-db.values:
	default:
		v = db.alloc()
	}
	return v
}

const (
	nameSplitter   = '='
	pairSplitter   = ','
	prefixSplitter = '+'
)

var (
	keyDB       = newKeyDB(512, 512, 64)
	emptyString = ""
)

type itemDB struct {
	bufferDB  *DB
	stringsDB *DB
}

func KeyMap(prefix string, stringMap map[string]string) string {
	keys := keyDB.stringsDB.get().([]string)
	for k := range stringMap {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	buf := keyDB.bufferDB.get().(*bytes.Buffer)

	if prefix != emptyString {
		buf.WriteString(prefix)
		buf.WriteByte(prefixSplitter)
	}

	sortedKeysLen := len(stringMap)
	for i := 0; i < sortedKeysLen; i++ {
		buf.WriteString(keys[i])
		buf.WriteByte(nameSplitter)
		buf.WriteString(stringMap[keys[i]])
		if i != sortedKeysLen-1 {
			buf.WriteByte(pairSplitter)
		}
	}

	key := buf.String()
	keyDB.release(buf, keys)
	return key
}

func newKeyDB(size, blen, slen int) *itemDB {
	b := newDB(size)
	b.init(func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, blen))
	})

	s := newDB(size)
	s.init(func() interface{} {
		return make([]string, 0, slen)
	})

	return &itemDB{
		bufferDB:  b,
		stringsDB: s,
	}
}

func (s *itemDB) release(b *bytes.Buffer, strs []string) {
	b.Reset()
	s.bufferDB.put(b)

	for i := range strs {
		strs[i] = emptyString
	}
	s.stringsDB.put(strs[:0])
}
