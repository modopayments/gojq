%{
package gojq

func reverseFuncDef(xs []*FuncDef) []*FuncDef {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
	return xs
}

func prependFuncDef(xs []*FuncDef, x *FuncDef) []*FuncDef {
	xs = append(xs, nil)
	copy(xs[1:], xs)
	xs[0] = x
	return xs
}
%}

%union {
  value    any
  token    string
  operator Operator
  pos      int
}

%type<value> program header imports import meta body funcdefs funcdef funcargs query
%type<value> bindpatterns pattern arraypatterns objectpatterns objectpattern
%type<value> expr term string stringparts suffix args ifelifs ifelse trycatch
%type<value> objectkeyvals objectkeyval objectval
%type<value> constterm constobject constobjectkeyvals constobjectkeyval constarray constarrayelems
%type<token> tokIdentVariable tokIdentModuleIdent tokVariableModuleVariable tokKeyword objectkey
%token<operator> tokAltOp tokUpdateOp tokDestAltOp tokCompareOp
%token<token> tokOrOp tokAndOp tokModule tokImport tokInclude tokDef tokAs tokLabel tokBreak
%token<token> tokNull tokTrue tokFalse
%token<token> tokIf tokThen tokElif tokElse tokEnd
%token<token> tokTry tokCatch tokReduce tokForeach
%token<token> tokIdent tokVariable tokModuleIdent tokModuleVariable
%token<token> tokRecurse tokIndex tokNumber tokFormat
%token<token> tokString tokStringStart tokStringQuery tokStringEnd
%token<token> tokInvalid tokInvalidEscapeSequence tokUnterminatedString

%nonassoc tokFuncDefQuery tokExpr tokTerm
%right '|'
%left ','
%right tokAltOp
%nonassoc tokUpdateOp
%left tokOrOp
%left tokAndOp
%nonassoc tokCompareOp
%left '+' '-'
%left '*' '/' '%'
%nonassoc tokAs tokIndex '.' '?' tokEmptyCatch
%nonassoc '[' tokTry tokCatch

%%

program
    : header imports body
    {
        query := $3.(*Query)
        query.Meta = $1.(*ConstObject)
        query.Imports = $2.([]*Import)
        l := yylex.(*lexer)
        query.Comments = l.comments
        l.result = query
    }

header
    :
    {
        $$ = (*ConstObject)(nil)
    }
    | tokModule constobject ';'
    {
        $$ = $2;
    }

imports
    :
    {
        $$ = []*Import(nil)
    }
    | imports import
    {
        $$ = append($1.([]*Import), $2.(*Import))
    }

import
    : tokImport tokString tokAs tokIdentVariable meta ';'
    {
        $$ = &Import{ImportPath: $2, ImportAlias: $4, Meta: $5.(*ConstObject)}
    }
    | tokInclude tokString meta ';'
    {
        $$ = &Import{IncludePath: $2, Meta: $3.(*ConstObject)}
    }

meta
    :
    {
        $$ = (*ConstObject)(nil)
    }
    | constobject

body
    : funcdefs
    {
        q := &Query{FuncDefs: reverseFuncDef($1.([]*FuncDef))}
        $$ = q
        $<pos>$ = $<pos>1
    }
    | query

funcdefs
    :
    {
        $$ = []*FuncDef(nil)
    }
    | funcdef funcdefs
    {
        $$ = append($2.([]*FuncDef), $1.(*FuncDef))
        $<pos>$ = $<pos>1
    }

funcdef
    : tokDef tokIdent ':' query ';'
    {
        $$ = &FuncDef{Name: $2, Body: $4.(*Query), Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokDef tokIdent '(' funcargs ')' ':' query ';'
    {
        $$ = &FuncDef{Name: $2, Args: $4.([]string), Body: $7.(*Query), Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }

funcargs
    : tokIdentVariable
    {
        $$ = []string{$1}
    }
    | funcargs ';' tokIdentVariable
    {
        $$ = append($1.([]string), $3)
    }

tokIdentVariable
    : tokIdent
    | tokVariable

query
    : funcdef query %prec tokFuncDefQuery
    {
        query := $2.(*Query)
        query.FuncDefs = prependFuncDef(query.FuncDefs, $1.(*FuncDef))
        $$ = query
        $<pos>$ = $<pos>1
    }
    | query '|' query
    {
        q := &Query{Left: $1.(*Query), Op: OpPipe, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $$ = q
        $<pos>$ = $<pos>1
    }
    | query tokAs bindpatterns '|' query
    {
        q := &Query{Left: $1.(*Query), Op: OpPipe, Right: $5.(*Query), Patterns: $3.([]*Pattern), Pos: $<pos>1, OpPos: $<pos>4}
        $$ = q
        $<pos>$ = $<pos>1
    }
    | tokLabel tokVariable '|' query
    {
        q := &Query{Term: &Term{Type: TermTypeLabel, Label: &Label{$2, $4.(*Query)}, Pos: $<pos>1}, Pos: $<pos>1}
        $$ = q
        $<pos>$ = $<pos>1
    }
    | query ',' query
    {
        q := &Query{Left: $1.(*Query), Op: OpComma, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $$ = q
        $<pos>$ = $<pos>1
    }
    | expr %prec tokExpr

expr
    : expr tokAltOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr tokUpdateOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr tokOrOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpOr, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr tokAndOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpAnd, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr tokCompareOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr '+' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpAdd, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr '-' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpSub, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr '*' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpMul, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr '/' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpDiv, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | expr '%' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpMod, Right: $3.(*Query), Pos: $<pos>1, OpPos: $<pos>2}
        $<pos>$ = $<pos>1
    }
    | term %prec tokTerm
    {
        t := $1.(*Term)
        $$ = &Query{Term: t, Pos: t.Pos}
        $<pos>$ = $<pos>1
    }

bindpatterns
    : pattern
    {
        $$ = []*Pattern{$1.(*Pattern)}
    }
    | bindpatterns tokDestAltOp pattern
    {
        $$ = append($1.([]*Pattern), $3.(*Pattern))
    }

pattern
    : tokVariable
    {
        $$ = &Pattern{Name: $1}
    }
    | '[' arraypatterns ']'
    {
        $$ = &Pattern{Array: $2.([]*Pattern)}
    }
    | '{' objectpatterns '}'
    {
        $$ = &Pattern{Object: $2.([]*PatternObject)}
    }

arraypatterns
    : pattern
    {
        $$ = []*Pattern{$1.(*Pattern)}
    }
    | arraypatterns ',' pattern
    {
        $$ = append($1.([]*Pattern), $3.(*Pattern))
    }

objectpatterns
    : objectpattern
    {
        $$ = []*PatternObject{$1.(*PatternObject)}
    }
    | objectpatterns ',' objectpattern
    {
        $$ = append($1.([]*PatternObject), $3.(*PatternObject))
    }

objectpattern
    : objectkey ':' pattern
    {
        $$ = &PatternObject{Key: $1, Val: $3.(*Pattern)}
    }
    | string ':' pattern
    {
        $$ = &PatternObject{KeyString: $1.(*String), Val: $3.(*Pattern)}
    }
    | '(' query ')' ':' pattern
    {
        $$ = &PatternObject{KeyQuery: $2.(*Query), Val: $5.(*Pattern)}
    }
    | tokVariable
    {
        $$ = &PatternObject{Key: $1}
    }

term
    : '.'
    {
        $$ = &Term{Type: TermTypeIdentity, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokRecurse
    {
        $$ = &Term{Type: TermTypeRecurse, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokIndex
    {
        $$ = &Term{Type: TermTypeIndex, Index: &Index{Name: $1}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '.' suffix
    {
        suffix := $2.(*Suffix)
        var t *Term
        if suffix.Iter {
            t = &Term{Type: TermTypeIdentity, SuffixList: []*Suffix{suffix}, Pos: $<pos>1}
        } else {
            t = &Term{Type: TermTypeIndex, Index: suffix.Index, Pos: $<pos>1}
        }
        $$ = t
        $<pos>$ = $<pos>1
    }
    | '.' string
    {
        $$ = &Term{Type: TermTypeIndex, Index: &Index{Str: $2.(*String)}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokNull
    {
        $$ = &Term{Type: TermTypeNull, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokTrue
    {
        $$ = &Term{Type: TermTypeTrue, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokFalse
    {
        $$ = &Term{Type: TermTypeFalse, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokIdentModuleIdent
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokIdentModuleIdent '(' args ')'
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1, Args: $3.([]*Query)}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokVariableModuleVariable
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '{' '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{ClosePos: $<pos>2}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '{' objectkeyvals '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{KeyVals: $2.([]*ObjectKeyVal), ClosePos: $<pos>3}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '{' objectkeyvals ',' '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{KeyVals: $2.([]*ObjectKeyVal), ClosePos: $<pos>4}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '[' ']'
    {
        $$ = &Term{Type: TermTypeArray, Array: &Array{ClosePos: $<pos>2}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '[' query ']'
    {
        $$ = &Term{Type: TermTypeArray, Array: &Array{Query: $2.(*Query), ClosePos: $<pos>3}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokNumber
    {
        $$ = &Term{Type: TermTypeNumber, Number: $1, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '+' term
    {
        $$ = &Term{Type: TermTypeUnary, Unary: &Unary{OpAdd, $2.(*Term)}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '-' term
    {
        $$ = &Term{Type: TermTypeUnary, Unary: &Unary{OpSub, $2.(*Term)}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokFormat
    {
        $$ = &Term{Type: TermTypeFormat, Format: $1, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokFormat string
    {
        $$ = &Term{Type: TermTypeFormat, Format: $1, Str: $2.(*String), Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | string
    {
        $$ = &Term{Type: TermTypeString, Str: $1.(*String), Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokIf query tokThen query ifelifs ifelse tokEnd
    {
        $$ = &Term{Type: TermTypeIf, If: &If{Cond: $2.(*Query), Then: $4.(*Query), ThenPos: $<pos>3, Elif: $5.([]*IfElif), Else: $6.(*Query), ElsePos: $<pos>6, EndPos: $<pos>7}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokTry expr trycatch
    {
        $$ = &Term{Type: TermTypeTry, Try: &Try{Body: $2.(*Query), Catch: $3.(*Query), CatchPos: $<pos>3}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokReduce expr tokAs pattern '(' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeReduce, Reduce: &Reduce{Query: $2.(*Query), Pattern: $4.(*Pattern), Start: $6.(*Query), Update: $8.(*Query), ClosePos: $<pos>9}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokForeach expr tokAs pattern '(' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeForeach, Foreach: &Foreach{Query: $2.(*Query), Pattern: $4.(*Pattern), Start: $6.(*Query), Update: $8.(*Query), Extract: nil, ClosePos: $<pos>9}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokForeach expr tokAs pattern '(' query ';' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeForeach, Foreach: &Foreach{Query: $2.(*Query), Pattern: $4.(*Pattern), Start: $6.(*Query), Update: $8.(*Query), Extract: $10.(*Query), ClosePos: $<pos>11}, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | tokBreak tokVariable
    {
        $$ = &Term{Type: TermTypeBreak, Break: $2, Pos: $<pos>1}
        $<pos>$ = $<pos>1
    }
    | '(' query ')'
    {
        $$ = &Term{Type: TermTypeQuery, Query: $2.(*Query), Pos: $<pos>1, ClosePos: $<pos>3}
        $<pos>$ = $<pos>1
    }
    | term tokIndex
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Index: &Index{Name: $2}})
        $<pos>$ = $<pos>1
    }
    | term suffix
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, $2.(*Suffix))
        $<pos>$ = $<pos>1
    }
    | term '?'
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Optional: true})
        $<pos>$ = $<pos>1
    }
    | term '.' suffix
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, $3.(*Suffix))
        $<pos>$ = $<pos>1
    }
    | term '.' string
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Index: &Index{Str: $3.(*String)}})
        $<pos>$ = $<pos>1
    }

string
    : tokString
    {
        $$ = &String{Str: $1}
    }
    | tokStringStart stringparts tokStringEnd
    {
        $$ = &String{Queries: $2.([]*Query)}
    }

stringparts
    :
    {
        $$ = []*Query{}
    }
    | stringparts tokString
    {
        $$ = append($1.([]*Query), &Query{Term: &Term{Type: TermTypeString, Str: &String{Str: $2}}})
    }
    | stringparts tokStringQuery query ')'
    {
        yylex.(*lexer).inString = true
        $$ = append($1.([]*Query), &Query{Term: &Term{Type: TermTypeQuery, Query: $3.(*Query)}})
    }

tokIdentModuleIdent
    : tokIdent
    | tokModuleIdent

tokVariableModuleVariable
    : tokVariable
    | tokModuleVariable

suffix
    : '[' ']'
    {
        $$ = &Suffix{Iter: true}
    }
    | '[' query ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query)}}
    }
    | '[' query ':' ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query), IsSlice: true}}
    }
    | '[' ':' query ']'
    {
        $$ = &Suffix{Index: &Index{End: $3.(*Query), IsSlice: true}}
    }
    | '[' query ':' query ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query), End: $4.(*Query), IsSlice: true}}
    }

args
    : query
    {
        $$ = []*Query{$1.(*Query)}
    }
    | args ';' query
    {
        $$ = append($1.([]*Query), $3.(*Query))
    }

ifelifs
    :
    {
        $$ = []*IfElif(nil)
    }
    | ifelifs tokElif query tokThen query
    {
        $$ = append($1.([]*IfElif), &IfElif{Cond: $3.(*Query), Then: $5.(*Query), Pos: $<pos>2, ThenPos: $<pos>4})
    }

ifelse
    :
    {
        $$ = (*Query)(nil)
    }
    | tokElse query
    {
        $$ = $2
        $<pos>$ = $<pos>1
    }

trycatch
    : %prec tokEmptyCatch
    {
        $$ = (*Query)(nil)
    }
    | tokCatch expr
    {
        $$ = $2
        $<pos>$ = $<pos>1
    }

objectkeyvals
    : objectkeyval
    {
        $$ = []*ObjectKeyVal{$1.(*ObjectKeyVal)}
    }
    | objectkeyvals ',' objectkeyval
    {
        $$ = append($1.([]*ObjectKeyVal), $3.(*ObjectKeyVal))
    }

objectkeyval
    : objectkey ':' objectval
    {
        $$ = &ObjectKeyVal{Key: $1, Val: $3.(*Query), Pos: $<pos>1}
    }
    | string ':' objectval
    {
        $$ = &ObjectKeyVal{KeyString: $1.(*String), Val: $3.(*Query), Pos: $<pos>1}
    }
    | '(' query ')' ':' objectval
    {
        $$ = &ObjectKeyVal{KeyQuery: $2.(*Query), Val: $5.(*Query), Pos: $<pos>1}
    }
    | objectkey
    {
        $$ = &ObjectKeyVal{Key: $1, Pos: $<pos>1}
    }
    | string
    {
        $$ = &ObjectKeyVal{KeyString: $1.(*String), Pos: $<pos>1}
    }

objectkey
    : tokIdent
    | tokVariable
    | tokKeyword

objectval
    : objectval '|' objectval
    {
        $$ = &Query{Left: $1.(*Query), Op: OpPipe, Right: $3.(*Query)}
    }
    | expr

constterm
    : constobject
    {
        $$ = &ConstTerm{Object: $1.(*ConstObject)}
    }
    | constarray
    {
        $$ = &ConstTerm{Array: $1.(*ConstArray)}
    }
    | tokNumber
    {
        $$ = &ConstTerm{Number: $1}
    }
    | tokString
    {
        $$ = &ConstTerm{Str: $1}
    }
    | tokNull
    {
        $$ = &ConstTerm{Null: true}
    }
    | tokTrue
    {
        $$ = &ConstTerm{True: true}
    }
    | tokFalse
    {
        $$ = &ConstTerm{False: true}
    }

constobject
    : '{' '}'
    {
        $$ = &ConstObject{}
    }
    | '{' constobjectkeyvals '}'
    {
        $$ = &ConstObject{$2.([]*ConstObjectKeyVal)}
    }
    | '{' constobjectkeyvals ',' '}'
    {
        $$ = &ConstObject{$2.([]*ConstObjectKeyVal)}
    }

constobjectkeyvals
    : constobjectkeyval
    {
        $$ = []*ConstObjectKeyVal{$1.(*ConstObjectKeyVal)}
    }
    | constobjectkeyvals ',' constobjectkeyval
    {
        $$ = append($1.([]*ConstObjectKeyVal), $3.(*ConstObjectKeyVal))
    }

constobjectkeyval
    : tokIdent ':' constterm
    {
        $$ = &ConstObjectKeyVal{Key: $1, Val: $3.(*ConstTerm)}
    }
    | tokKeyword ':' constterm
    {
        $$ = &ConstObjectKeyVal{Key: $1, Val: $3.(*ConstTerm)}
    }
    | tokString ':' constterm
    {
        $$ = &ConstObjectKeyVal{KeyString: $1, Val: $3.(*ConstTerm)}
    }

constarray
    : '[' ']'
    {
        $$ = &ConstArray{}
    }
    | '[' constarrayelems ']'
    {
        $$ = &ConstArray{$2.([]*ConstTerm)}
    }

constarrayelems
    : constterm
    {
        $$ = []*ConstTerm{$1.(*ConstTerm)}
    }
    | constarrayelems ',' constterm
    {
        $$ = append($1.([]*ConstTerm), $3.(*ConstTerm))
    }

tokKeyword
    : tokOrOp
    | tokAndOp
    | tokModule
    | tokImport
    | tokInclude
    | tokDef
    | tokAs
    | tokLabel
    | tokBreak
    | tokNull
    | tokTrue
    | tokFalse
    | tokIf
    | tokThen
    | tokElif
    | tokElse
    | tokEnd
    | tokTry
    | tokCatch
    | tokReduce
    | tokForeach

%%
