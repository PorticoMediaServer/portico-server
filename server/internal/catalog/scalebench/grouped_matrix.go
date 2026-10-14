package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"portico.local/server/internal/catalog"
)

// Open actual subgroups with their entity pivot's complete vocabulary. Group
// listings alone do not measure the title pages reached from those listings.
func exerciseGroupedVocabulary(db *sql.DB, s *catalog.Service, v catalog.Viewer, add func(string, func(context.Context, *catalog.Service) error) error) error {
	type group struct {
		name, library, pivot string
		query                catalog.BrowseNode
	}
	groups := []group{}
	var genre string
	if err := db.QueryRow(`SELECT label FROM catalog_terms WHERE vocab=1 ORDER BY id LIMIT 1`).Scan(&genre); err != nil {
		return err
	}
	groups = append(groups, group{"genre", "movies", "movies", catalog.BrowseNode{Field: "genre", Operator: "contains", Value: genre}})
	for _, entry := range []struct{ name, library, pivot, sql string }{
		{"show", "tv", "episodes", `SELECT pid(e.public_id) FROM catalog_shows sh JOIN catalog_entities e ON e.id=sh.entity_id ORDER BY (SELECT count(*) FROM catalog_episodes ep WHERE ep.show_id=sh.entity_id) DESC LIMIT 1`},
		{"artist", "music", "songs", `SELECT pid(e.public_id) FROM catalog_artists artist JOIN catalog_entities e ON e.id=artist.entity_id ORDER BY (SELECT count(*) FROM catalog_song_artists link WHERE link.artist_id=artist.entity_id) DESC LIMIT 1`},
	} {
		var public string
		if err := db.QueryRow(entry.sql).Scan(&public); err != nil {
			return err
		}
		groups = append(groups, group{entry.name, entry.library, entry.pivot, catalog.BrowseNode{Field: "memberOf", Operator: "equals", Value: public}})
	}
	for _, g := range groups {
		caps, err := s.BrowseCapabilitiesFor(g.library, g.pivot)
		if err != nil {
			return err
		}
		measure := func(name string, query *catalog.BrowseNode, sort []catalog.BrowseSortSelection) error {
			for _, start := range []int{0, 50000} {
				suffix := "first"
				if start > 0 {
					suffix = "deep"
				}
				request := catalog.BrowseRequest{Library: g.library, Pivot: g.pivot, Query: query, Sort: sort, Limit: 40}
				if start > 0 {
					request.Range = &catalog.BrowseRange{Start: start}
				}
				if err := add("grouped/"+g.name+"/"+name+"/"+suffix, func(_ context.Context, service *catalog.Service) error {
					_, err := service.BrowseEntities(v, request)
					return err
				}); err != nil {
					return err
				}
			}
			return nil
		}
		for _, sort := range caps.Sorts {
			for _, direction := range sort.Directions {
				selection := catalog.BrowseSortSelection{Field: sort.ID, Direction: direction}
				if sort.ID == "random" {
					selection.Seed = 2000000000
				}
				if err := measure("sort/"+sort.ID+"/"+direction, &g.query, []catalog.BrowseSortSelection{selection}); err != nil {
					return err
				}
			}
		}
		for _, field := range caps.Fields {
			node := catalog.BrowseNode{Field: field.ID, Operator: field.Operators[0], Value: "Title"}
			switch field.Type {
			case "number":
				node.Value = float64(2000)
			case "date":
				node.Value = "2026-01-01"
			case "boolean":
				node.Value = true
			case "enum":
				if len(field.AllowedValues) > 0 {
					node.Value = field.AllowedValues[0]
				}
			}
			for _, op := range field.Operators {
				if op == "is-present" {
					node.Operator = op
					node.Value = nil
					break
				}
			}
			combined := catalog.BrowseNode{All: []catalog.BrowseNode{g.query, node}}
			raw, _ := json.Marshal(combined)
			parsed, err := catalog.ParseBrowseQuery(raw, "query")
			if err != nil {
				return fmt.Errorf("grouped %s/%s: %w", g.name, field.ID, err)
			}
			if err := measure("filter/"+field.ID, parsed, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
