package analysis

import (
	"context"
	"strings"
	"testing"

	"codegraph/internal/models"
)

func TestGoAnalyzerSymbolAndRelationshipEvidence(t *testing.T) {
	content := `package auth

import (
	"fmt"
	"github.com/go-chi/chi/v5"
)

type AuthService struct {
	ID string
}

func (a *AuthService) Login(user string) bool {
	fmt.Println(user)
	return validateUser(user)
}

func validateUser(user string) bool {
	return user == "admin"
}
`

	ga := NewGoAnalyzer()
	res, err := ga.Analyze(context.Background(), "repo-1", "file-1", "src/auth/auth.go", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Verify Symbols & Evidence
	symMap := make(map[string]*models.Symbol)
	for _, sym := range res.Symbols {
		symMap[sym.Name] = sym
	}

	authStruct, ok := symMap["AuthService"]
	if !ok || authStruct.Kind != models.SymbolKindStruct || authStruct.Location.StartLine != 8 {
		t.Errorf("AuthService struct symbol error: %+v", authStruct)
	}

	loginMethod, ok := symMap["Login"]
	if !ok || loginMethod.Kind != models.SymbolKindMethod || loginMethod.Location.StartLine != 12 {
		t.Errorf("Login method symbol error: %+v", loginMethod)
	}

	validateFunc, ok := symMap["validateUser"]
	if !ok || validateFunc.Kind != models.SymbolKindFunction || validateFunc.Location.StartLine != 17 {
		t.Errorf("validateUser function symbol error: %+v", validateFunc)
	}

	// 2. Verify Imports & External Target Evidence
	var chiImport *models.Relationship
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeImports && rel.TargetID == "github.com/go-chi/chi/v5" {
			chiImport = rel
			break
		}
	}
	if chiImport == nil || chiImport.TargetKind != models.TargetKindExternal {
		t.Fatalf("expected external import for chi, got: %+v", chiImport)
	}

	// 3. Verify Calls Resolution Evidence
	var validateCall, fmtCall *models.Relationship
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeCalls {
			if rel.TargetID == validateFunc.ID {
				validateCall = rel
			}
			if rel.TargetID == "fmt.Println" {
				fmtCall = rel
			}
		}
	}

	if validateCall == nil || validateCall.Status != models.RelStatusResolved {
		t.Errorf("expected RESOLVED status for validateUser call, got: %+v", validateCall)
	}
	if fmtCall == nil || fmtCall.Status != models.RelStatusUnresolved {
		t.Errorf("expected UNRESOLVED status for fmt.Println selector call, got: %+v", fmtCall)
	}
}

func TestJSTSAnalyzerEvidence(t *testing.T) {
	content := `import express from 'express';
import { db } from './db';

export function handleLogin(req, res) {
	const user = req.body.user;
	return db.getUser(user);
}
`

	analyzer := NewJSTSAnalyzer("TypeScript")
	res, err := analyzer.Analyze(context.Background(), "repo-1", "file-2", "src/login.ts", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify external import
	var extImp *models.Relationship
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeImports && rel.TargetID == "express" {
			extImp = rel
		}
	}
	if extImp == nil || extImp.TargetKind != models.TargetKindExternal {
		t.Errorf("expected external import for express, got %+v", extImp)
	}

	// Verify symbol location
	if len(res.Symbols) == 0 || res.Symbols[0].Name != "handleLogin" || res.Symbols[0].Location.StartLine != 4 {
		t.Errorf("expected handleLogin symbol on line 4, got %+v", res.Symbols)
	}
}

func TestPythonAnalyzerEvidence(t *testing.T) {
	content := `import os
from .database import Client

class UserService(Client):
    def get_user(self, user_id):
        return self.fetch(user_id)
`

	pyAnalyzer := NewPythonAnalyzer()
	res, err := pyAnalyzer.Analyze(context.Background(), "repo-1", "file-3", "services/user.py", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify class and inheritance
	var clsSym *models.Symbol
	var extRel *models.Relationship
	for _, sym := range res.Symbols {
		if sym.Name == "UserService" && sym.Kind == models.SymbolKindClass {
			clsSym = sym
		}
	}
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeExtends {
			extRel = rel
		}
	}

	if clsSym == nil || clsSym.Location.StartLine != 4 {
		t.Errorf("expected UserService class on line 4, got %+v", clsSym)
	}
	if extRel == nil || extRel.TargetID != "Client" || extRel.Status != models.RelStatusPartial {
		t.Errorf("expected EXTENDS Client relationship, got %+v", extRel)
	}
}

func TestPythonAnalyzerModuleLevelAssignments(t *testing.T) {
	content := `import os
from crewai import Agent

search_tool = SerperDevTool()

research_agent = Agent(
    role="Senior AI Researcher",
    tools=[search_tool]
)
`

	pyAnalyzer := NewPythonAnalyzer()
	res, err := pyAnalyzer.Analyze(context.Background(), "repo-1", "file-4", "agents.py", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	symMap := make(map[string]*models.Symbol)
	for _, sym := range res.Symbols {
		symMap[sym.Name] = sym
	}

	toolSym, okTool := symMap["search_tool"]
	if !okTool || toolSym.Kind != models.SymbolKindVariable || toolSym.Location.StartLine != 4 {
		t.Errorf("expected search_tool variable on line 4, got: %+v", toolSym)
	}

	agentSym, okAgent := symMap["research_agent"]
	if !okAgent || agentSym.Kind != models.SymbolKindVariable || agentSym.Location.StartLine != 6 {
		t.Errorf("expected research_agent variable on line 6, got: %+v", agentSym)
	}

	// Verify relationship connecting research_agent to search_tool
	var useRel *models.Relationship
	for _, rel := range res.Relationships {
		if rel.SourceID == agentSym.ID && rel.TargetID == toolSym.ID {
			useRel = rel
			break
		}
	}

	if useRel == nil || useRel.Status != models.RelStatusResolved {
		t.Errorf("expected RESOLVED relationship between research_agent and search_tool, got: %+v", useRel)
	}
}

func TestPythonAnalyzer_CrossFileImportsAndReferences(t *testing.T) {
	content := `import crew as c
from crew import research_crew as rc
from tools import search_tool
import os

research_agent = Agent(
    role="Analyst",
    tools=[search_tool]
)

research_task = Task(
    agent=research_agent
)

def main():
    rc.kickoff()

if __name__ == "__main__":
    main()
`

	pa := NewPythonAnalyzer()
	res, err := pa.Analyze(context.Background(), "repo-py", "run.py", "run.py", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Test local vs external import parsing & aliases
	var crewImport, osImport *models.Relationship
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeImports {
			if strings.Contains(rel.TargetID, "crew") {
				crewImport = rel
			}
			if rel.TargetID == "os" {
				osImport = rel
			}
		}
	}

	if crewImport == nil || crewImport.TargetKind != models.TargetKindInternal {
		t.Errorf("expected internal import for local module 'crew', got: %+v", crewImport)
	}
	if osImport == nil || osImport.TargetKind != models.TargetKindExternal {
		t.Errorf("expected external import for stdlib 'os', got: %+v", osImport)
	}

	// 2. Test Argument-level & List-level dependency extraction
	var agentToolRel, taskAgentRel *models.Relationship
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeCalls {
			if strings.Contains(rel.TargetID, "search_tool") {
				agentToolRel = rel
			}
			if strings.Contains(rel.TargetID, "research_agent") {
				taskAgentRel = rel
			}
		}
	}

	if agentToolRel == nil {
		t.Errorf("expected relationship from Agent tools list to search_tool")
	}
	if taskAgentRel == nil {
		t.Errorf("expected relationship from Task agent kwarg to research_agent")
	}

	// 3. Test main guard detection warning
	hasMainGuard := false
	for _, w := range res.Warnings {
		if w == "MAIN_GUARD_DETECTED" {
			hasMainGuard = true
		}
	}
	if !hasMainGuard {
		t.Errorf("expected MAIN_GUARD_DETECTED warning for if __name__ == '__main__': block")
	}
}
