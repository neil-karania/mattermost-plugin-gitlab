// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package webhook

import (
	"context"
	"fmt"

	"github.com/xanzy/go-gitlab"
)

func (w *webhook) HandleIssueComment(ctx context.Context, event *gitlab.IssueCommentEvent) ([]*HandleWebhook, []string, error) {
	var warnings []string
	handlers, err := w.handleDMIssueComment(event)
	if err != nil {
		return nil, warnings, err
	}
	handlers2, warnings, err := w.handleChannelIssueComment(ctx, event)
	if err != nil {
		return nil, warnings, err
	}
	return cleanWebhookHandlers(append(handlers, handlers2...)), warnings, nil
}

func (w *webhook) handleDMIssueComment(event *gitlab.IssueCommentEvent) ([]*HandleWebhook, error) {
	senderGitlabUsername := event.User.Username
	message := fmt.Sprintf("[%s](%s) commented on your issue [%s#%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.Project.PathWithNamespace, event.Issue.IID, event.ObjectAttributes.URL)

	toUsers := make([]string, len(event.Issue.AssigneeIDs)+1)
	for index, assigneeID := range event.Issue.AssigneeIDs {
		toUsers[index] = w.gitlabRetreiver.GetUsernameByID(assigneeID)
	}
	toUsers[len(toUsers)-1] = w.gitlabRetreiver.GetUsernameByID(event.Issue.AuthorID)

	handlers := []*HandleWebhook{
		{
			Message: message,
			ToUsers: toUsers,
			From:    senderGitlabUsername,
		},
	}

	if mention := w.handleMention(mentionDetails{
		senderUsername:    senderGitlabUsername,
		pathWithNamespace: event.Project.PathWithNamespace,
		IID:               fmt.Sprintf("%d", event.Issue.IID),
		URL:               event.ObjectAttributes.URL,
		body:              event.ObjectAttributes.Description,
	}); mention != nil {
		handlers = append(handlers, mention)
	}

	return handlers, nil
}

func (w *webhook) handleChannelIssueComment(ctx context.Context, event *gitlab.IssueCommentEvent) ([]*HandleWebhook, []string, error) {
	senderGitlabUsername := event.User.Username
	repo := event.Project
	body := event.ObjectAttributes.Description
	res := []*HandleWebhook{}

	message := fmt.Sprintf("[%s](%s) New comment by [%s](%s) on [#%v %s](%s):\n\n%s", repo.PathWithNamespace, repo.WebURL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.Issue.IID, event.Issue.Title, event.ObjectAttributes.URL, body)

	toChannels := make([]string, 0)
	namespace, project := normalizeNamespacedProject(repo.PathWithNamespace)
	subs := w.gitlabRetreiver.GetSubscribedChannelsForProject(
		ctx, namespace, project,
		repo.Visibility == gitlab.PublicVisibility,
	)
	var warnings []string
	for _, sub := range subs {
		if !sub.IssueComments() {
			continue
		}

		ok, warning := anyEventLabelInSubs(sub, event.Issue.Labels)
		if !ok {
			if len(warning) > 0 {
				warnings = append(warnings, warning)
			}
			continue
		}

		toChannels = append(toChannels, sub.ChannelID)
	}
	if len(toChannels) > 0 {
		res = append(res, &HandleWebhook{
			From:       senderGitlabUsername,
			Message:    message,
			ToUsers:    []string{},
			ToChannels: toChannels,
		})
	}
	return res, warnings, nil
}

func (w *webhook) HandleMergeRequestComment(ctx context.Context, event *gitlab.MergeCommentEvent) ([]*HandleWebhook, []string, error) {
	var warnings []string
	handlers, err := w.handleDMMergeRequestComment(event)
	if err != nil {
		return nil, warnings, err
	}
	handlers2, warnings, err := w.handleChannelMergeRequestComment(ctx, event)
	if err != nil {
		return nil, warnings, err
	}
	return cleanWebhookHandlers(append(handlers, handlers2...)), warnings, nil
}

func (w *webhook) handleDMMergeRequestComment(event *gitlab.MergeCommentEvent) ([]*HandleWebhook, error) {
	senderGitlabUsername := event.User.Username
	message := fmt.Sprintf("[%s](%s) commented on your merge request [%s#%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.Project.PathWithNamespace, event.MergeRequest.IID, event.ObjectAttributes.URL)

	handlers := []*HandleWebhook{{
		Message: message,
		ToUsers: []string{w.gitlabRetreiver.GetUsernameByID(event.MergeRequest.AssigneeID), w.gitlabRetreiver.GetUsernameByID(event.MergeRequest.AuthorID)},
		From:    senderGitlabUsername,
	}}

	if mention := w.handleMention(mentionDetails{
		senderUsername:    senderGitlabUsername,
		pathWithNamespace: event.Project.PathWithNamespace,
		IID:               fmt.Sprintf("%d", event.MergeRequest.IID),
		URL:               event.ObjectAttributes.URL,
		body:              event.ObjectAttributes.Description,
	}); mention != nil {
		handlers = append(handlers, mention)
	}
	return handlers, nil
}

func (w *webhook) handleChannelMergeRequestComment(ctx context.Context, event *gitlab.MergeCommentEvent) ([]*HandleWebhook, []string, error) {
	senderGitlabUsername := event.User.Username
	repo := event.Project
	body := event.ObjectAttributes.Description
	res := []*HandleWebhook{}

	message := fmt.Sprintf("[%s](%s) New comment by [%s](%s) on [#%v %s](%s):\n\n%s", repo.PathWithNamespace, repo.WebURL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.MergeRequest.IID, event.MergeRequest.Title, event.ObjectAttributes.URL, body)
	var warnings []string
	toChannels := make([]string, 0)
	namespace, project := normalizeNamespacedProject(repo.PathWithNamespace)
	subs := w.gitlabRetreiver.GetSubscribedChannelsForProject(
		ctx, namespace, project,
		repo.Visibility == gitlab.PublicVisibility,
	)
	for _, sub := range subs {
		if !sub.MergeRequestComments() {
			continue
		}

		ok, warning := anyEventLabelInSubs(sub, event.MergeRequest.Labels)
		if !ok {
			if len(warning) > 0 {
				warnings = append(warnings, warning)
			}
			continue
		}

		toChannels = append(toChannels, sub.ChannelID)
	}
	if len(toChannels) > 0 {
		res = append(res, &HandleWebhook{
			From:       senderGitlabUsername,
			Message:    message,
			ToUsers:    []string{},
			ToChannels: toChannels,
		})
	}
	return res, warnings, nil
}
