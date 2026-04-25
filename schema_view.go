package jubako

import (
	"reflect"

	"github.com/yacchi/jubako/layer"
)

type storeSchemaView struct {
	schema *Schema
}

func newStoreSchemaView(schema *Schema) layer.SchemaView {
	return storeSchemaView{schema: schema}
}

func (v storeSchemaView) Lookup(path string) (layer.PathDescriptor, bool) {
	if v.schema == nil || v.schema.Trie == nil {
		return nil, false
	}
	mapping := v.schema.Trie.Lookup(path)
	if mapping == nil {
		return nil, false
	}
	return storePathDescriptor{mapping: mapping, path: path}, true
}

func (v storeSchemaView) Descriptors() []layer.PathDescriptor {
	if v.schema == nil || len(v.schema.Mappings) == 0 {
		return nil
	}

	descriptors := make([]layer.PathDescriptor, 0, len(v.schema.Mappings))
	for _, mapping := range v.schema.Mappings {
		if mapping == nil || mapping.Path == "" {
			continue
		}
		descriptors = append(descriptors, storePathDescriptor{
			mapping: mapping,
			path:    mapping.Path,
		})
	}
	return descriptors
}

type storePathDescriptor struct {
	mapping *PathMapping
	path    string
}

func (d storePathDescriptor) Path() string {
	return d.path
}

func (d storePathDescriptor) FieldKey() string {
	if d.mapping == nil {
		return ""
	}
	return d.mapping.FieldKey
}

func (d storePathDescriptor) Sensitive() bool {
	return d.mapping != nil && d.mapping.Sensitive == sensitiveExplicit
}

func (d storePathDescriptor) Tag(key string) (string, bool) {
	if d.mapping == nil {
		return "", false
	}
	value, ok := d.mapping.StructField.Tag.Lookup(key)
	return value, ok
}

func (d storePathDescriptor) StructField() reflect.StructField {
	if d.mapping == nil {
		return reflect.StructField{}
	}
	return d.mapping.StructField
}
