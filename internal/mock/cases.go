package mock

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/scenario"
	"github.com/inhere/fakeserver/internal/tpl"
)

// RespondCases handles one request for a route with cases[]. The pipeline is:
//
//  1. build template ctx from the request (once, shared with Respond)
//  2. evaluate each case's matcher against the ctx; collect (OrigIdx, Weight)
//     for those returning true. Matcher runtime errors → stderr warn + skip.
//  3. selector picks one OrigIdx from the filtered set. Empty set → 500 +
//     {"error":"no case matched", ...}.
//  4. construct a virtual *config.Route from the chosen case, inheriting
//     status/delay/headers from the outer route when the case omits them,
//     and delegate to Respond — which already implements the full §4.5
//     rendering pipeline.
//
// matchers must be parallel to route.Cases (one Matcher per case, even for
// cases without a when clause — the empty Matcher matches unconditionally).
// selector is owned by the caller (one per route).
func RespondCases(c *rux.Context, route *config.Route, routeIndex int, matchers []*Matcher, selector Selector, renderer tpl.Renderer, envMap map[string]any) {
	RespondCasesWithScenario(c, nil, route, routeIndex, matchers, selector, renderer, envMap, nil, "")
}

func RespondCasesWithScenario(
	c *rux.Context,
	cfg *config.Config,
	route *config.Route,
	routeIndex int,
	matchers []*Matcher,
	selector Selector,
	renderer tpl.Renderer,
	envMap map[string]any,
	scenarioStore *scenario.Store,
	cliScenario string,
) {
	scenarioName, scenarioSource := scenario.Resolve(c.Req, scenarioStore, cfg, cliScenario)
	ctx := tpl.BuildRenderCtx(c.Req, paramsFromContext(c), nil, envMap)
	routeKey := scenario.NewRouteKey(c.Req.Method, route.Path)
	recordRouteTrace(c, route, routeIndex, "cases", nil)

	if scenarioStore != nil {
		if ov, ok := scenarioStore.ConsumeOverride(routeKey); ok {
			if idx, found := pickCaseByName(route.Cases, ov.CaseName); found {
				respondPickedCase(c, route, routeIndex, idx, "override:"+ov.Mode, scenarioName, renderer, envMap, ctx)
				return
			}
		}
	}

	if scenarioName != "" && cfg != nil {
		if sc, ok := cfg.Scenarios[scenarioName]; ok {
			if caseName := sc.Routes[routeKey.String()]; caseName != "" {
				if idx, found := pickCaseByName(route.Cases, caseName); found {
					respondPickedCase(c, route, routeIndex, idx, scenarioSource, scenarioName, renderer, envMap, ctx)
					return
				}
			}
		}
	}

	filtered := make([]SelectorCase, 0, len(route.Cases))
	var unmatched []string
	var whenErrs []whenCaseError
	for i, m := range matchers {
		ok, err := m.Evaluate(ctx)
		if err != nil {
			log.Printf("[mock] %s %s case[%d] when err: %v (skipping)", joinMethods(route), route.Path, i, err)
			whenErrs = append(whenErrs, whenCaseError{Case: caseName(&route.Cases[i], i), Error: err.Error()})
			continue
		}
		if !ok {
			unmatched = append(unmatched, caseName(&route.Cases[i], i))
			continue
		}
		filtered = append(filtered, SelectorCase{OrigIdx: i, Weight: route.Cases[i].Weight})
	}
	if len(whenErrs) > 0 {
		recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{WhenError: formatWhenErrors(whenErrs)})
	}

	pickedIdx, err := selector.Pick(filtered)
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			writeNoCaseMatched(c.Resp, route, unmatched, whenErrs)
			return
		}
		writeError(c.Resp, http.StatusInternalServerError, "selector error", err.Error(), route)
		return
	}

	respondPickedCase(c, route, routeIndex, pickedIdx, "strategy", scenarioName, renderer, envMap, ctx)
}

func respondPickedCase(c *rux.Context, route *config.Route, routeIndex int, caseIndex int, source string, scenarioName string, renderer tpl.Renderer, envMap map[string]any, ctx map[string]any) {
	chosen := &route.Cases[caseIndex]
	recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{
		RouteIndex:     &routeIndex,
		CaseIndex:      &caseIndex,
		RouteMode:      "cases",
		RouteSource:    route.SourceFile,
		Scenario:       scenarioName,
		CaseName:       chosen.Name,
		OverrideSource: source,
	})
	virtual := caseAsRoute(route, chosen)
	respondWithTrace(c, virtual, routeIndex, "cases", &caseIndex, renderer, envMap, ctx)
}

// caseAsRoute composes a virtual single-response Route by overlaying the
// chosen case onto the outer route's defaults (design §3.2 末尾).
func caseAsRoute(outer *config.Route, c *config.RouteCase) *config.Route {
	v := &config.Route{
		Method:     outer.Method,
		Path:       outer.Path,
		SourceFile: outer.SourceFile, // keeps bodyFile resolution working
		Status:     c.Status,
		Delay:      c.Delay,
		Headers:    c.Headers,
		Body:       c.Body,
		BodyFile:   c.BodyFile,
		Paginate:   c.Paginate,
	}
	if v.Status == 0 {
		v.Status = outer.Status
	}
	if v.Delay == "" {
		v.Delay = outer.Delay
	}
	if len(v.Headers) == 0 && len(outer.Headers) > 0 {
		v.Headers = outer.Headers
	} else if len(v.Headers) > 0 && len(outer.Headers) > 0 {
		// merge: outer keys not in case are added; case wins on conflict
		merged := make(map[string]string, len(outer.Headers)+len(v.Headers))
		for k, val := range outer.Headers {
			merged[k] = val
		}
		for k, val := range v.Headers {
			merged[k] = val
		}
		v.Headers = merged
	}
	if v.Body == nil && v.BodyFile == "" {
		v.Body = outer.Body
		v.BodyFile = outer.BodyFile
	}
	if v.Paginate == nil {
		v.Paginate = outer.Paginate
	}
	return v
}

// whenCaseError is one case-level when-expression failure, rendered in the
// "no case matched" 500 body so a silent skip never hides a broken when.
type whenCaseError struct {
	Case  string `json:"case"`
	Error string `json:"error"`
}

// formatWhenErrors renders when failures for the access log / history marker:
// "<case>:<err>", multiple joined by "; ".
func formatWhenErrors(errs []whenCaseError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Case+":"+e.Error)
	}
	return strings.Join(parts, "; ")
}

// caseName returns a case's display name for diagnostics/500 bodies, falling
// back to its index for unnamed cases.
func caseName(cs *config.RouteCase, idx int) string {
	if cs == nil || cs.Name == "" {
		return fmt.Sprintf("#%d", idx)
	}
	return cs.Name
}

func joinMethods(r *config.Route) string {
	if len(r.Method) == 0 {
		return "?"
	}
	if len(r.Method) == 1 {
		return r.Method[0]
	}
	out := r.Method[0]
	for _, m := range r.Method[1:] {
		out += "," + m
	}
	return out
}
