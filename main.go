package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sashabaranov/go-openai"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func main() {
	repoPath := "." // カレントディレクトリをリポジトリパスとして使用
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		log.Fatalf("リポジトリを開けませんでした: %v", err)
	}

	diff, err := getStagedDiff(repo)
	if err != nil {
		log.Fatalf("ステージングされたdiffを取得できませんでした: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY 環境変数が設定されていません")
	}

	client := openai.NewClient(apiKey)
	if err != nil {
		log.Fatalf("OpenAIクライアントの作成に失敗しました: %v", err)
	}

	commitMessage, err := generateCommitMessage(client, diff)
	if err != nil {
		log.Fatalf("コミットメッセージを生成できませんでした: %v", err)
	}

	fmt.Println(commitMessage)
}

func getStagedDiff(repo *git.Repository) (string, error) {
	w, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	head, err := repo.Head()
	if err != nil {
		return "", err
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", err
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return "", err
	}

	status, err := w.Status()
	if err != nil {
		return "", err
	}

	dmp := diffmatchpatch.New()

	var diffStr string
	for filePath, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified {
			oldFile, err := commitTree.File(filePath)
			if err != nil && err != object.ErrFileNotFound {
				return "", err
			}

			newFile, err := w.Filesystem.Open(filePath)
			if err != nil {
				return "", err
			}

			oldContent := ""
			if oldFile != nil {
				oldContentBytes, err := oldFile.Contents()
				if err != nil {
					return "", err
				}
				oldContent = string(oldContentBytes)
			}

			newContentBytes, err := io.ReadAll(newFile)
			if err != nil {
				return "", err
			}
			newContent := string(newContentBytes)

			diffs := dmp.DiffMain(oldContent, newContent, false)
			diffStr += fmt.Sprintf("index 0644\n--- a/%s\n+++ b/%s\n%s",
				filePath, filePath, dmp.DiffPrettyText(diffs)) + "\n"
		}
	}

	return diffStr, nil
}

func generateCommitMessage(client *openai.Client, diff string) (string, error) {
	prompt := fmt.Sprintf("Write a commit message in Japanese describing the changes and the reasons for them\n\n%s", diff)
	request := &openai.ChatCompletionRequest{
		Model:     openai.GPT3Dot5Turbo,
		Messages:  []openai.ChatCompletionMessage{{Role: "user", Content: prompt}},
		MaxTokens: 100,
	}

	ctx := context.Background()
	response, err := client.CreateChatCompletion(ctx, *request)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no suggestions received")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}
