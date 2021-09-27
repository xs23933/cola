package cola

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xs23933/uid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Pages struct {
	P, L  int
	Total int64
	Data  interface{}
}

type Model struct {
	ID        uid.UID `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *gorm.DeletedAt `json:",omitempty"`
}

func (m *Model) BeforeCreate(tx *DB) error {
	if m.ID.IsEmpty() {
		m.ID = uid.New()
	}
	return nil
}

func NewModel(dsn string, debug bool) (*DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if debug {
		db = db.Debug()
	}
	Conn = db
	return db, err
}

// 字典类型

// Dict map[string]interface{}
type Dict map[string]interface{}

type Map = map[string]interface{}

// Value 数据驱动接口
func (d Dict) Value() (driver.Value, error) {
	bytes, err := json.Marshal(d)
	return string(bytes), err
}

// Scan 数据驱动接口
func (d *Dict) Scan(src interface{}) error {
	switch val := src.(type) {
	case string:
		return json.Unmarshal([]byte(val), d)
	case []byte:
		if strings.EqualFold(string(val), "null") {
			*d = make(Dict)
			return nil
		}
		if err := json.Unmarshal(val, d); err != nil {
			*d = make(Dict)
		}
		return nil
	}
	return fmt.Errorf("not support %s", src)
}

// GormDataType schema.Field DataType
func (Dict) GormDataType() string {
	return "text"
}

/** 数组部分 **/

// Array 数组类型
type Array []interface{}

// Value 数据驱动接口
func (d Array) Value() (driver.Value, error) {
	bytes, err := json.Marshal(d)
	return string(bytes), err
}

// Scan 数据驱动接口
func (d *Array) Scan(src interface{}) error {
	switch val := src.(type) {
	case string:
		return json.Unmarshal([]byte(val), d)
	case []byte:
		if strings.EqualFold(string(val), "null") {
			*d = Array{}
			return nil
		}
		if err := json.Unmarshal(val, d); err != nil {
			*d = Array{}
		}
		return nil
	}
	return fmt.Errorf("not support %s", src)
}

// Strings 转换为 []string
func (d Array) String() []string {
	arr := make([]string, 0)
	for _, v := range d {
		arr = append(arr, fmt.Sprint(v))
	}
	return arr
}

// StringsJoin 链接为字符串
func (d Array) StringsJoin(sp string) string {
	arr := d.String()
	return strings.Join(arr, sp)
}

// GormDataType schema.Field DataType
func (Array) GormDataType() string {
	return "text"
}

// 空字符串 存入数据库 存 NULL ，这样会跳过数据库唯一索引的检查
type StringOrNil string

// implements driver.Valuer, will be invoked automatically when written to the db
func (s StringOrNil) Value() (driver.Value, error) {
	if s == "" {
		return nil, nil
	}
	return []byte(s), nil
}

// implements sql.Scanner, will be invoked automatically when read from the db
func (s *StringOrNil) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		*s = StringOrNil(v)
	case []byte:
		*s = StringOrNil(v)
	case nil:
		*s = ""
	}
	return nil
}

func (s StringOrNil) String() string {
	return string(s)
}

// ToHandle 转换名称为handle
func ToHandle(src string) string {
	src = strings.ToLower(src)
	r := strings.NewReplacer("^", "", "`", "", "~", "", "!", "", "@", "", "#", "", "%", "", "|", "", "\\", "", "[&]", "", "]", "", "/", "", "&", "", "--", "-", " ", "-", "'", "")
	return r.Replace(src)
}

func Where(whr *map[string]interface{}, db ...*DB) (*DB, int, int) {
	var tx *DB
	if len(db) > 0 {
		tx = db[0]
	} else {
		tx = Conn
	}

	wher := *whr

	l, ok := wher["l"].(float64)
	lmt := 20
	if ok {
		delete(wher, "l") //删除lmt
		lmt = int(l)
	}

	tx = tx.Limit(lmt)
	p, ok := wher["p"].(float64)
	pos := 1
	if ok {
		pos = int(p)
		delete(wher, "p") // 删除 pos
		tx = tx.Offset((pos - 1) * lmt)
	}

	desc, ok := wher["desc"].(string)
	if ok {
		delete(wher, "desc")
		tx = tx.Order(fmt.Sprintf("%s desc", strings.Replace(desc, ",", " ", -1)))
	}
	asc, ok := wher["asc"].(string)
	if ok {
		delete(wher, "asc")
		tx = tx.Order(strings.Replace(asc, ",", " ", -1))
	}

	if name, ok := wher["name"]; ok {
		delete(wher, "name")
		if name != "" {
			tx = tx.Where("name like ?", fmt.Sprintf("%%%s%%", name))
		}
	}

	// 过滤掉字符串等于空 的搜索
	if len(wher) > 0 {
		for k, v := range wher {
			if x, ok := v.(string); ok && x == "" {
				delete(wher, k)
			}
			if strings.HasSuffix(k, " !=") {
				tx = tx.Where(fmt.Sprintf("`%s` <> ?", strings.TrimSuffix(k, " !=")), v)
				delete(wher, k)
			}
			if strings.HasSuffix(k, " >") || strings.HasSuffix(k, " <") {
				ks := strings.Split(k, " ")
				ks[0] = fmt.Sprintf("`%s`", ks[0])
				ks = append(ks, "?")
				tx = tx.Where(strings.Join(ks, " "), v)
				delete(wher, k)
			}
		}
		tx = tx.Where(wher)
	}
	return tx, pos, lmt
}

var (
	Conn *DB
)

type DB = gorm.DB
