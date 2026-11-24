// Package all lists Rumblestrip's analyzers, for rumblestrip-vet and rumblestrip check alike.
package all

import (
	"github.com/McReaper/rumblestrip-go/internal/analyzers/boundedlist"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/comments"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/ctxroot"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/domainconv"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/goroutine"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/logreturn"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/nesting"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/pkgname"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/sqlconst"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/testassert"
	"github.com/McReaper/rumblestrip-go/internal/analyzers/typedid"
	"golang.org/x/tools/go/analysis"
)

// Analyzers returns a new instance of every analyzer of Rumblestrip.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		boundedlist.New(),
		comments.New(),
		ctxroot.New(),
		domainconv.New(),
		goroutine.New(),
		logreturn.New(),
		nesting.New(),
		pkgname.New(),
		sqlconst.New(),
		testassert.New(),
		typedid.New(),
	}
}
