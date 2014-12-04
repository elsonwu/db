/*
  Copyright (c) 2012-2014 José Carlos Nieto, https://menteslibres.net/xiam

  Permission is hereby granted, free of charge, to any person obtaining
  a copy of this software and associated documentation files (the
  "Software"), to deal in the Software without restriction, including
  without limitation the rights to use, copy, modify, merge, publish,
  distribute, sublicense, and/or sell copies of the Software, and to
  permit persons to whom the Software is furnished to do so, subject to
  the following conditions:

  The above copyright notice and this permission notice shall be
  included in all copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
  EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
  MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
  NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
  LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
  OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
  WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package mongo

import (
	"fmt"
	"strings"

	"github.com/elsonwu/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// Mongodb Collection
type Collection struct {
	name       string
	parent     *Source
	collection *mgo.Collection
}

type chunks struct {
	Fields     []string
	Limit      int
	Offset     int
	Sort       []string
	Conditions interface{}
	GroupBy    []interface{}
}

func (self *Collection) Find(terms ...interface{}) db.Result {

	queryChunks := &chunks{}

	// No specific fields given.
	if len(queryChunks.Fields) == 0 {
		queryChunks.Fields = []string{"*"}
	}

	queryChunks.Conditions = self.compileQuery(terms...)

	if debugEnabled() == true {
		debugLogQuery(queryChunks)
	}

	// Actually executing query.
	result := &Result{
		self,
		queryChunks,
		nil,
	}

	return result
}

// Transforms conditions into something *mgo.Session can understand.
func compileStatement(cond db.Cond) bson.M {
	conds := bson.M{}

	// Walking over conditions
	for field, value := range cond {
		// Removing leading or trailing spaces.
		field = strings.TrimSpace(field)

		chunks := strings.SplitN(field, ` `, 2)

		var op string

		if len(chunks) > 1 {
			switch chunks[1] {
			case `>`:
				op = `$gt`
			case `<`:
				op = `$lt`
			case `<=`:
				op = `$lte`
			case `>=`:
				op = `$gte`
			default:
				op = chunks[1]
			}
		}

		switch value := value.(type) {
		case db.Func:
			conds[chunks[0]] = bson.M{value.Name: value.Args}
		default:
			if op == "" {
				conds[chunks[0]] = value
			} else {
				conds[chunks[0]] = bson.M{op: value}
			}
		}

	}

	return conds
}

// Compiles terms into something *mgo.Session can understand.
func (self *Collection) compileConditions(term interface{}) interface{} {

	switch t := term.(type) {
	case []interface{}:
		values := []interface{}{}
		for i, _ := range t {
			value := self.compileConditions(t[i])
			if value != nil {
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			return values
		}
	case db.Or:
		values := []interface{}{}
		for i, _ := range t {
			values = append(values, self.compileConditions(t[i]))
		}
		condition := bson.M{`$or`: values}
		return condition
	case db.And:
		values := []interface{}{}
		for i, _ := range t {
			values = append(values, self.compileConditions(t[i]))
		}
		condition := bson.M{`$and`: values}
		return condition
	case db.Cond:
		return compileStatement(t)
	}
	return nil
}

// Compiles terms into something that *mgo.Session can understand.
func (self *Collection) compileQuery(terms ...interface{}) interface{} {
	var query interface{}

	compiled := self.compileConditions(terms)

	if compiled != nil {
		conditions := compiled.([]interface{})
		if len(conditions) == 1 {
			query = conditions[0]
		} else {
			// this should be correct.
			// query = map[string]interface{}{"$and": conditions}

			// trying to workaround https://jira.mongodb.org/browse/SERVER-4572
			mapped := map[string]interface{}{}
			for _, v := range conditions {
				for kk, _ := range v.(map[string]interface{}) {
					mapped[kk] = v.(map[string]interface{})[kk]
				}
			}

			query = mapped
		}
	} else {
		query = map[string]interface{}{}
	}

	return query
}

func (self *Collection) Name() string {
	return self.collection.Name
}

// Deletes all the rows within the collection.
func (self *Collection) Truncate() error {
	err := self.collection.DropCollection()

	if err != nil {
		return err
	}

	return nil
}

// Appends an item (map or struct) into the collection.
func (self *Collection) Append(item interface{}) (interface{}, error) {
	var err error
	var id bson.ObjectId

	id = bson.NewObjectId()

	// Allocating a new ID.
	if err = self.collection.Insert(bson.M{"_id": id}); err != nil {
		return nil, err
	}

	// Now append data the user wants to append.
	if err = self.collection.Update(bson.M{"_id": id}, item); err != nil {
		return nil, err
	}

	return id, nil

	/*
		var id bson.ObjectId
		var err error

		id = bson.NewObjectId()

		_, err = self.collection.Upsert(bson.M{"_id": id}, item);

		if err != nil {
			return nil, err
		}

		return id, nil
	*/
}

// Returns true if the collection exists.
func (self *Collection) Exists() bool {
	query := self.parent.database.C(`system.namespaces`).Find(map[string]string{`name`: fmt.Sprintf(`%s.%s`, self.parent.database.Name, self.collection.Name)})
	count, _ := query.Count()
	if count > 0 {
		return true
	}
	return false
}

// Transforms data from db.Item format into mgo format.
func toInternal(val interface{}) interface{} {

	// TODO: use reflection to target kinds and not just types.
	switch t := val.(type) {
	case db.Cond:
		for k, _ := range t {
			t[k] = toInternal(t[k])
		}
	case map[string]interface{}:
		for k, _ := range t {
			t[k] = toInternal(t[k])
		}
	}

	return val
}

// Transforms data from mgo format into db.Item format.
func toNative(val interface{}) interface{} {

	// TODO: use reflection to target kinds and not just types.

	switch t := val.(type) {
	case bson.M:
		v := map[string]interface{}{}
		for i, _ := range t {
			v[i] = toNative(t[i])
		}
		return v
	}

	return val

}
