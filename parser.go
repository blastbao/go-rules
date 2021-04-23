package gorules

import (
	"errors"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"
)

// 错误定义
var (
	ErrRuleEmpty      = errors.New("rule is empty")
	ErrTypeNotStruct  = errors.New("value must struct or struct pointer")
	ErrNotFoundTag    = errors.New("not found tag")
	ErrUnsupportToken = errors.New("unsupport token")
	ErrUnsupportExpr  = errors.New("unsupport expr")
	ErrNotNumber      = errors.New("not a number")
	ErrNotBool        = errors.New("not boolean")
)

// Bool 规则 rule 结果的布尔值，rule 的参数基于 base 的 json tag
func Bool(base interface{}, rule string) (bool, error) {
	r, err := NewRule(rule)
	if err != nil {
		return false, err
	}
	return r.Bool(base)
}

// Int 规则 rule 的结果如果是数值型，转换为 int64 ，否则报错
func Int(base interface{}, rule string) (int64, error) {
	r, err := NewRule(rule)
	if err != nil {
		return 0, err
	}
	return r.Int(base)
}

// Float 返回规则 rule 结果，如果数值型返回 float64 ，否则报错
func Float(base interface{}, rule string) (float64, error) {
	r, err := NewRule(rule)
	if err != nil {
		return 0, err
	}
	return r.Float(base)
}

// 拆解 rule ，支持的计算类型 +、-、*、/、&&、||，其他报错
// 支持二元操作

// 从 struct 解析找到 json Tag , 若嵌套 struct 则用 “.” 连接
func getValueByTag(x reflect.Value, tag string) (interface{}, error) {
	// 指针解引用
	if x.Kind() == reflect.Ptr {
		x = x.Elem()
	}
	// 检查 x 是否为结构体类型
	if x.Kind() != reflect.Struct {
		return x, ErrTypeNotStruct
	}
	// 获取 x 的结构体定义
	t := x.Type()
	// 遍历 x 的结构体字段
	for i := 0; i < t.NumField(); i++ {
		// 获取当前字段的 rule 标识
		js := getTagName(t.Field(i).Tag)
		// 如果当前字段的 rule 标识和预查询的 tag 匹配，就返回该字段值
		if js == tag {
			return x.Field(i).Interface(), nil
		}
	}
	return nil, ErrNotFoundTag
}

// 获取数组元素
func getSliceValue(x reflect.Value, idx int) (interface{}, error) {
	// 检查 x 是否为 slice 或 array 类型
	if x.Kind() != reflect.Slice && x.Kind() != reflect.Array {
		return nil, errors.New("only slice or array can get value by index")
	}
	// 检查下标越界
	if idx > x.Len()-1 {
		return nil, errors.New("slice index out of range")
	}
	// 取第 idx 个元素
	return x.Index(idx).Interface(), nil
}


//
func getValue(base reflect.Value, expr ast.Expr) (interface{}, error) {

	nullValue := reflect.Value{}

	switch t := expr.(type) {

	// 二元表达式: 比较运算、逻辑运算、数值运算
	case *ast.BinaryExpr:

		// 左表达式求值
		x, err := getValue(base, t.X)
		if err != nil {
			return nullValue, err
		}

		// 右表达式求值
		y, err := getValue(base, t.Y)
		if err != nil {
			return nullValue, err
		}

		// 二元操作
		return operate(x, y, t.Op)

	// 标识符
	case *ast.Ident:
		return getValueByTag(base, t.Name)

	//
	case *ast.BasicLit:
		switch t.Kind {
		case token.STRING:
			return strings.Trim(t.Value, "\""), nil
		case token.INT:
			return strconv.ParseInt(t.Value, 10, 64)
		case token.FLOAT:
			return strconv.ParseFloat(t.Value, 64)
		default:
			return nullValue, errors.New("unsupport param")
		}

	//
	case *ast.ParenExpr:
		return getValue(base, t.X)

	//
	case *ast.SelectorExpr:
		v, err := getValue(base, t.X)
		if err != nil {
			return nullValue, err
		}
		return getValueByTag(reflect.ValueOf(v), t.Sel.Name)

	//
	case *ast.IndexExpr:

		// 获取索引下标 idx
		idx, err := getValue(base, t.Index)
		if err != nil {
			return nullValue, err
		}

		// 检查 idx 数据类型
		f, ok := idx.(float64)
		if !ok {
			if i, ok := idx.(int64); ok {
				f = float64(i)
			} else {
				return nullValue, errors.New("index must be int or float")
			}
		}

		//
		v, err := getValue(base, t.X)
		if err != nil {
			return nullValue, err
		}

		return getSliceValue(reflect.ValueOf(v), int(f))

	// 函数表达式
	case *ast.CallExpr:
		//
		if fexp, ok := t.Fun.(*ast.Ident); ok {
			// 如果函数名是 "IN"
			if strings.ToUpper(fexp.Name) == "IN" {
				// 参数列表长度为 2
				if len(t.Args) == 2 {
					// 执行 IsIn 操作
					return isIn(base, t.Args[0], t.Args[1])
				}
				return nullValue, errors.New("function IN only support tow params")
			}
			return nullValue, errors.New("unsupport function: " + fexp.Name)
		}
		return nullValue, errors.New("unknow function")
	default:
		return nullValue, ErrUnsupportExpr
	}
}

func isIn(base reflect.Value, slice ast.Expr, key ast.Expr) (bool, error) {

	sv, err := getValue(base, slice)
	if err != nil {
		return false, err
	}

	kv, err := getValue(base, key)
	if err != nil {
		return false, err
	}

	svv := reflect.ValueOf(sv)
	if svv.Kind() != reflect.Slice && svv.Kind() != reflect.Array {
		return false, errors.New("function IN first param must be slice or array")
	}

	if svv.Len() == 0 {
		return false, nil
	}

	kvv := reflect.ValueOf(kv)
	switch svv.Index(0).Kind() {
	case reflect.String:
		for i := 0; i < svv.Len(); i++ {
			if svv.Index(i).String() == kvv.String() {
				return true, nil
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		for i := 0; i < svv.Len(); i++ {
			if svv.Index(i).Int() == kvv.Int() {
				return true, nil
			}
		}
	case reflect.Float32, reflect.Float64:
		for i := 0; i < svv.Len(); i++ {
			if svv.Index(i).Float() == kvv.Float() {
				return true, nil
			}
		}
	default:
		return false, errors.New("function IN only support: string int float")
	}
	return false, nil
}
