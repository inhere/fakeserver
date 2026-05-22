package mock

import (
	"errors"
	"log"
	"net/http"

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
//
// Known limitation (Phase 4 boundary): BuildRenderCtx reads req.Body. After
// this call req.Body is drained but BuildRenderCtx caches bodyRaw, so the
// second call inside Respond will see an empty body. The practical effect:
// when-expressions CAN reference .request.body (they execute first and see
// the parsed body), but case body/headers templates cannot. Phase 5 may
// revisit this with an explicit context.WithValue or body-buffering layer.
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
	routeKey := scenario.NewRouteKey(firstMethod(route), route.Path)
	recordRouteTrace(c, route, routeIndex, "cases", nil)

	if scenarioStore != nil {
		if ov, ok := scenarioStore.ConsumeOverride(routeKey); ok {
			if idx, found := pickCaseByName(route.Cases, ov.CaseName); found {
				respondPickedCase(c, route, routeIndex, idx, "override:"+ov.Mode, scenarioName, renderer, envMap)
				return
			}
		}
	}

	if scenarioName != "" && cfg != nil {
		if sc, ok := cfg.Scenarios[scenarioName]; ok {
			if caseName := sc.Routes[routeKey.String()]; caseName != "" {
				if idx, found := pickCaseByName(route.Cases, caseName); found {
					respondPickedCase(c, route, routeIndex, idx, scenarioSource, scenarioName, renderer, envMap)
					return
				}
			}
		}
	}

	ctx := tpl.BuildRenderCtx(c.Req, paramsFromContext(c), nil, envMap)

	filtered := make([]SelectorCase, 0, len(route.Cases))
	for i, m := range matchers {
		ok, err := m.Evaluate(ctx)
		if err != nil {
			log.Printf("[mock] %s %s case[%d] when err: %v (skipping)", joinMethods(route), route.Path, i, err)
			continue
		}
		if !ok {
			continue
		}
		filtered = append(filtered, SelectorCase{OrigIdx: i, Weight: route.Cases[i].Weight})
	}

	pickedIdx, err := selector.Pick(filtered)
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			writeError(c.Resp, http.StatusInternalServerError, "no case matched", "", route)
			return
		}
		writeError(c.Resp, http.StatusInternalServerError, "selector error", err.Error(), route)
		return
	}

	respondPickedCase(c, route, routeIndex, pickedIdx, "strategy", scenarioName, renderer, envMap)
}

func respondPickedCase(c *rux.Context, route *config.Route, routeIndex int, caseIndex int, source string, scenarioName string, renderer tpl.Renderer, envMap map[string]any) {
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
	respondWithTrace(c, virtual, routeIndex, "cases", &caseIndex, renderer, envMap)
}

func firstMethod(route *config.Route) string {
	if route == nil || len(route.Method) == 0 {
		return ""
	}
	return route.Method[0]
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
	return v
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
