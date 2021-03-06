package main

import (

	// "log"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
)

var (
	// ErrNoCheckedOutBranch : When we have no checked out branch
	ErrNoCheckedOutBranch = errors.New("No currently checked out branch")
)

// GitFile : A staged/unstaged file
// TODO: decide whether to give all of these the Git prefix
type GitFile struct {
	Name               string
	HasStagedChanges   bool
	HasUnstagedChanges bool
	Tracked            bool
	Deleted            bool
	HasMergeConflicts  bool
	DisplayString      string
}

// Branch : A git branch
type Branch struct {
	Name          string
	Type          string
	BaseBranch    string
	DisplayString string
}

// Commit : A git commit
type Commit struct {
	Sha           string
	Name          string
	Pushed        bool
	DisplayString string
}

// StashEntry : A git stash entry
type StashEntry struct {
	Index         int
	Name          string
	DisplayString string
}

// Map (from https://gobyexample.com/collection-functions)
func Map(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}

func includesString(list []string, a string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// not sure how to genericise this because []interface{} doesn't accept e.g.
// []int arguments
func includesInt(list []int, a int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func mergeGitStatusFiles(oldGitFiles, newGitFiles []GitFile) []GitFile {
	if len(oldGitFiles) == 0 {
		return newGitFiles
	}

	appendedIndexes := make([]int, 0)

	// retain position of files we already could see
	result := make([]GitFile, 0)
	for _, oldGitFile := range oldGitFiles {
		for newIndex, newGitFile := range newGitFiles {
			if oldGitFile.Name == newGitFile.Name {
				result = append(result, newGitFile)
				appendedIndexes = append(appendedIndexes, newIndex)
				break
			}
		}
	}

	// append any new files to the end
	for index, newGitFile := range newGitFiles {
		if !includesInt(appendedIndexes, index) {
			result = append(result, newGitFile)
		}
	}

	return result
}

func runDirectCommand(command string) (string, error) {
	timeStart := time.Now()

	commandLog(command)
	cmdOut, err := exec.
		Command("bash", "-c", command).
		CombinedOutput()
	devLog("run direct command time for command: ", command, time.Now().Sub(timeStart))

	return string(cmdOut), err
}

func branchStringParts(branchString string) (string, string) {
	// expect string to be something like '4w    master`
	splitBranchName := strings.Split(branchString, "\t")
	// if we have no \t then we have no recency, so just output that as blank
	if len(splitBranchName) == 1 {
		return "", branchString
	}
	return splitBranchName[0], splitBranchName[1]
}

// branchPropertiesFromName : returns branch type, base, and color
func branchPropertiesFromName(name string) (string, string, color.Attribute) {
	if strings.Contains(name, "feature/") {
		return "feature", "develop", color.FgGreen
	} else if strings.Contains(name, "bugfix/") {
		return "bugfix", "develop", color.FgYellow
	} else if strings.Contains(name, "hotfix/") {
		return "hotfix", "master", color.FgRed
	}
	return "other", name, color.FgWhite
}

func coloredString(str string, colour *color.Color) string {
	return colour.SprintFunc()(fmt.Sprint(str))
}

func withPadding(str string, padding int) string {
	if padding-len(str) < 0 {
		return str
	}
	return str + strings.Repeat(" ", padding-len(str))
}

func branchFromLine(line string, index int) Branch {
	recency, name := branchStringParts(line)
	branchType, branchBase, colourAttr := branchPropertiesFromName(name)
	if index == 0 {
		recency = "  *"
	}
	colour := color.New(colourAttr)
	displayString := withPadding(recency, 4) + coloredString(name, colour)
	return Branch{
		Name:          name,
		Type:          branchType,
		BaseBranch:    branchBase,
		DisplayString: displayString,
	}
}

func getGitBranches() []Branch {
	branches := make([]Branch, 0)
	// check if there are any branches
	branchCheck, _ := runDirectCommand("git branch")
	if branchCheck == "" {
		return append(branches, branchFromLine("master", 0))
	}
	rawString, _ := runDirectCommand(getBranchesCommand)
	branchLines := splitLines(rawString)
	for i, line := range branchLines {
		branches = append(branches, branchFromLine(line, i))
	}
	branches = getAndMergeFetchedBranches(branches)
	return branches
}

func branchAlreadyStored(branchLine string, branches []Branch) bool {
	for _, branch := range branches {
		if branch.Name == branchLine {
			return true
		}
	}
	return false
}

// here branches contains all the branches that we've checked out, along with
// the recency. In this function we append the branches that are in our heads
// directory i.e. things we've fetched but haven't necessarily checked out.
// Worth mentioning this has nothing to do with the 'git merge' operation
func getAndMergeFetchedBranches(branches []Branch) []Branch {
	rawString, _ := runDirectCommand(getHeadsCommand)
	branchLines := splitLines(rawString)
	for _, line := range branchLines {
		if branchAlreadyStored(line, branches) {
			continue
		}
		branches = append(branches, branchFromLine(line, len(branches)))
	}
	return branches
}

// TODO: DRY up this function and getGitBranches
func getGitStashEntries() []StashEntry {
	stashEntries := make([]StashEntry, 0)
	rawString, _ := runDirectCommand("git stash list --pretty='%gs'")
	for i, line := range splitLines(rawString) {
		stashEntries = append(stashEntries, stashEntryFromLine(line, i))
	}
	return stashEntries
}

func stashEntryFromLine(line string, index int) StashEntry {
	return StashEntry{
		Name:          line,
		Index:         index,
		DisplayString: line,
	}
}

func getStashEntryDiff(index int) (string, error) {
	return runCommand("git stash show -p --color stash@{" + fmt.Sprint(index) + "}")
}

func includes(array []string, str string) bool {
	for _, arrayStr := range array {
		if arrayStr == str {
			return true
		}
	}
	return false
}

func getGitStatusFiles() []GitFile {
	statusOutput, _ := getGitStatus()
	statusStrings := splitLines(statusOutput)
	gitFiles := make([]GitFile, 0)

	for _, statusString := range statusStrings {
		change := statusString[0:2]
		stagedChange := change[0:1]
		unstagedChange := statusString[1:2]
		filename := statusString[3:]
		tracked := !includes([]string{"??", "A "}, change)
		gitFile := GitFile{
			Name:               filename,
			DisplayString:      statusString,
			HasStagedChanges:   !includes([]string{" ", "U", "?"}, stagedChange),
			HasUnstagedChanges: unstagedChange != " ",
			Tracked:            tracked,
			Deleted:            unstagedChange == "D" || stagedChange == "D",
			HasMergeConflicts:  change == "UU",
		}
		devLog("tracked", gitFile.Tracked)
		devLog("hasUnstagedChanges", gitFile.HasUnstagedChanges)
		devLog("HasStagedChanges", gitFile.HasStagedChanges)
		devLog("DisplayString", gitFile.DisplayString)
		gitFiles = append(gitFiles, gitFile)
	}
	devLog(gitFiles)
	return gitFiles
}

func gitStashDo(index int, method string) (string, error) {
	return runCommand("git stash " + method + " stash@{" + fmt.Sprint(index) + "}")
}

func gitStashSave(message string) (string, error) {
	output, err := runCommand("git stash save \"" + message + "\"")
	if err != nil {
		return output, err
	}
	// if there are no local changes to save, the exit code is 0, but we want
	// to raise an error
	if output == "No local changes to save\n" {
		return output, errors.New(output)
	}
	return output, nil
}

func gitCheckout(branch string, force bool) (string, error) {
	forceArg := ""
	if force {
		forceArg = "--force "
	}
	return runCommand("git checkout " + forceArg + branch)
}

func runCommand(command string) (string, error) {
	commandStartTime := time.Now()
	commandLog(command)
	splitCmd := strings.Split(command, " ")
	cmdOut, err := exec.Command(splitCmd[0], splitCmd[1:]...).CombinedOutput()
	devLog("run command time: ", time.Now().Sub(commandStartTime))
	return string(cmdOut), err
}

func openFile(filename string) (string, error) {
	return runCommand("open " + filename)
}

func vsCodeOpenFile(filename string) (string, error) {
	return runCommand("code -r " + filename)
}

func sublimeOpenFile(filename string) (string, error) {
	return runCommand("subl " + filename)
}

func getBranchDiff(branch string, baseBranch string) (string, error) {

	return runCommand("git log -p -30 --color --no-merges " + branch)
	// return runCommand("git diff --color " + baseBranch + "..." + branch)
}

func verifyInGitRepo() {
	if output, err := runCommand("git status"); err != nil {
		fmt.Println(output)
		os.Exit(1)
	}
}

func getCommits() []Commit {
	pushables := gitCommitsToPush()
	log := getLog()
	commits := make([]Commit, 0)
	// now we can split it up and turn it into commits
	lines := splitLines(log)
	for _, line := range lines {
		splitLine := strings.Split(line, " ")
		sha := splitLine[0]
		pushed := includesString(pushables, sha)
		commits = append(commits, Commit{
			Sha:           sha,
			Name:          strings.Join(splitLine[1:], " "),
			Pushed:        pushed,
			DisplayString: strings.Join(splitLine, " "),
		})
	}
	return commits
}

func getLog() string {
	// currently limiting to 30 for performance reasons
	// TODO: add lazyloading when you scroll down
	result, err := runDirectCommand("git log --oneline -30")
	if err != nil {
		// assume if there is an error there are no commits yet for this branch
		return ""
	}
	return result
}

func gitIgnore(filename string) {
	if _, err := runDirectCommand("echo '" + filename + "' >> .gitignore"); err != nil {
		panic(err)
	}
}

func gitShow(sha string) string {
	result, err := runDirectCommand("git show --color " + sha)
	if err != nil {
		panic(err)
	}
	return result
}

func getDiff(file GitFile) string {
	cachedArg := ""
	if file.HasStagedChanges && !file.HasUnstagedChanges {
		cachedArg = "--cached "
	}
	deletedArg := ""
	if file.Deleted {
		deletedArg = "-- "
	}
	trackedArg := ""
	if !file.Tracked && !file.HasStagedChanges {
		trackedArg = "--no-index /dev/null "
	}
	command := "git diff --color " + cachedArg + deletedArg + trackedArg + file.Name
	// for now we assume an error means the file was deleted
	s, _ := runCommand(command)
	return s
}

func catFile(file string) (string, error) {
	return runDirectCommand("cat " + file)
}

func stageFile(file string) error {
	_, err := runCommand("git add " + file)
	return err
}

func unStageFile(file string, tracked bool) error {
	var command string
	if tracked {
		command = "git reset HEAD "
	} else {
		command = "git rm --cached "
	}
	devLog(command)
	_, err := runCommand(command + file)
	return err
}

func getGitStatus() (string, error) {
	return runCommand("git status --untracked-files=all --short")
}

func isInMergeState() (bool, error) {
	output, err := runCommand("git status --untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "conclude merge") || strings.Contains(output, "unmerged paths"), nil
}

func removeFile(file GitFile) error {
	// if the file isn't tracked, we assume you want to delete it
	if !file.Tracked {
		_, err := runCommand("rm -rf ./" + file.Name)
		return err
	}
	// if the file is tracked, we assume you want to just check it out
	_, err := runCommand("git checkout " + file.Name)
	return err
}

func gitCommit(message string) error {
	_, err := runDirectCommand("git commit -m \"" + message + "\"")
	return err
}

func gitPull() (string, error) {
	return runDirectCommand("git pull --no-edit")
}

func gitPush() (string, error) {
	branchName := gitCurrentBranchName()
	if branchName == "" {
		return "", ErrNoCheckedOutBranch
	}
	return runDirectCommand("git push -u origin " + branchName)
}

func gitSquashPreviousTwoCommits(message string) (string, error) {
	return runDirectCommand("git reset --soft head^ && git commit --amend -m \"" + message + "\"")
}

func gitRenameCommit(message string) (string, error) {
	return runDirectCommand("git commit --allow-empty --amend -m \"" + message + "\"")
}

func gitFetch() (string, error) {
	return runDirectCommand("git fetch")
}

func gitResetToCommit(sha string) (string, error) {
	return runDirectCommand("git reset " + sha)
}

func gitNewBranch(name string) (string, error) {
	return runDirectCommand("git checkout -b " + name)
}

func gitListStash() (string, error) {
	return runDirectCommand("git stash list")
}

func gitMerge(branchName string) (string, error) {
	return runDirectCommand("git merge --no-edit " + branchName)
}

func gitAbortMerge() (string, error) {
	return runDirectCommand("git merge --abort")
}

func gitUpstreamDifferenceCount() (string, string) {
	pushableCount, err := runDirectCommand("git rev-list @{u}..head --count")
	if err != nil {
		return "?", "?"
	}
	pullableCount, err := runDirectCommand("git rev-list head..@{u} --count")
	if err != nil {
		return "?", "?"
	}
	return strings.TrimSpace(pushableCount), strings.TrimSpace(pullableCount)
}

func gitCommitsToPush() []string {
	pushables, err := runDirectCommand("git rev-list @{u}..head --abbrev-commit")
	if err != nil {
		return make([]string, 0)
	}
	return splitLines(pushables)
}

func gitCurrentBranchName() string {
	branchName, err := runDirectCommand("git symbolic-ref --short HEAD")
	// if there is an error, assume there are no branches yet
	if err != nil {
		return ""
	}
	return branchName
}

const getBranchesCommand = `set -e
git reflog -n100 --pretty='%cr|%gs' --grep-reflog='checkout: moving' HEAD | {
  seen=":"
  git_dir="$(git rev-parse --git-dir)"
  while read line; do
    date="${line%%|*}"
    branch="${line##* }"
    if ! [[ $seen == *:"${branch}":* ]]; then
      seen="${seen}${branch}:"
      if [ -f "${git_dir}/refs/heads/${branch}" ]; then
        printf "%s\t%s\n" "$date" "$branch"
      fi
    fi
  done \
  | sed 's/ months /m /g' \
  | sed 's/ month /m /g' \
  | sed 's/ days /d /g' \
  | sed 's/ day /d /g' \
  | sed 's/ weeks /w /g' \
  | sed 's/ week /w /g' \
  | sed 's/ hours /h /g' \
  | sed 's/ hour /h /g' \
  | sed 's/ minutes /m /g' \
  | sed 's/ minute /m /g' \
  | sed 's/ seconds /s /g' \
  | sed 's/ second /s /g' \
  | sed 's/ago//g' \
  | tr -d ' '
}
`

const getHeadsCommand = `git show-ref \
| grep 'refs/heads/\|refs/remotes/origin/' \
| sed 's/.*refs\/heads\///g' \
| sed 's/.*refs\/remotes\/origin\///g' \
| grep -v '^HEAD$' \
| sort \
| uniq`
