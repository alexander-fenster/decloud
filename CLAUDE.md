# Instructions for Decloud project

## Code style

1. The language of implementation for both client and server is Golang.

2. The code must be formatted with `gofmt`.

3. Command line flags are to be parsed with [Cobra](https://github.com/spf13/cobra),
   YAML configuration with [Viper](https://github.com/spf13/viper).
   The preferred format for configuration files is TOML.

4. Unit tests are to be written with [Testify](https://github.com/stretchr/testify),
   use [Gomock](https://github.com/uber-go/mock) if a mock is needed.
   Do not create "change-detector tests".

5. Do not write obvious comments in the code.

## Process

**CRITICAL: NO CODING AT TOP LEVEL!**

Our star agentic team:

* Don Melton (the tech lead)
* Joel Spolsky (the implementation planner)
* Kent Beck (the test engineer)
* Rob Pike (the implementation engineer)
* Donald Knuth (advanced problem solver)
* Kevlin Henney (low-level reviewer)
* Linus Torvalds (high-level reviewer)
* Raymond Chen (the doc writer)
* Ward Cunningham (knowledge librarian)
* Andy Grove (HR and manager of agents)

We use a task-based workflow to ensure thorough planning, implementation, and review. 

Access to the tasks is performed via the Bureau MCP commands as described in the agent description files.
After each step, a commit is made; agents are encouraged to look at `git diff` to understand what was
changed on the previous steps of the workflow.

WE ALWAYS USE SUBAGENTS! THE TOP-LEVEL AGENT ONLY CALLS ON SUBAGENTS!

WORKFLOW - STEP 1 - SAVE REQUEST:

1. User's initial request is saved to the task file.
2. A git branch is created to work on the task.

WORKFLOW - STEP 2 - PLAN:

1. Don analyzes the codebase etc and creates the plan. The plan is committed to the branch.
2. Joel expands Don's plan with technical details and creates the tech plan. The tech plan is committed to the branch.
3. Linus reviews Don's plan and Joel's tech plan. The resulting files are committed to the branch.
4. Don, Joel and Linus iterate until Linus approves the plan, repeating all the steps. After each step, a commit to the branch is made.

WORKFLOW - STEP 3 - EXECUTION:

1. Kent writes tests in the appropriate package in the codebase (see Test Location Rules below) and creates a report in task dir. If stuck, call Donald Knuth. At the end of the step, a commit to the branch is made.
2. Rob implements code changes in the codebase and creates a report in task dir. If stuck, call Donald Knuth. At the end of the step, a commit to the branch is made.
3. Raymond updates the docs: API docs in \_docs/, AI docs in \_ai/. Creates report. The report is committed to the branch.
4. Kevlin and Linus review the changes in parallel. Important: any API docs updates MUST be reviewed for hallucinations by Kevlin very very carefully. All changed files are committed to the branch.
5. Go back to PLAN step so that Don reviews all results, Joel again expands, and Linus reviews. If ALL THREE (Don, Joel, Linus) agree that the task is FULLY DONE, then we're done, otherwise they iterate to come up with a new plan (as PLAN step explains) and then move back to EXECUTION. When any files or reports are changed, they are committed to the branch.

WORKFLOW - STEP 4 - FINALIZATION:

1. Ward preserves all the new learnings from these tasks for future reference. The result is committed to the branch.
2. If user asked for any corrections during this task, Andy considers updating agent instructions to align them to the user's intent. This is a VERY HIGH BAR, because agent definitions should not be updated lightly.
3. The git branch that has all the commits related to the task is **squash-merged** into the `main` branch with a conventional commit title and description.

**IMPORTANT:** PLAN step ALWAYS follows after EXECUTION. Each time we run Kent, Rob, Raymond, or Kevlin, we then must run PLAN step - Don, Joel and Linus.

**CRITICAL: NO CODING AT TOP LEVEL!**
