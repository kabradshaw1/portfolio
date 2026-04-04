"use client";

import { useQuery, useMutation } from "@apollo/client/react";
import { gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const GET_COMMENTS = gql`
  query TaskComments($taskId: ID!) {
    taskComments(taskId: $taskId) {
      id
      authorId
      body
      createdAt
    }
  }
`;

const ADD_COMMENT = gql`
  mutation AddComment($taskId: ID!, $body: String!) {
    addComment(taskId: $taskId, body: $body) {
      id
      body
    }
  }
`;

export function CommentSection({ taskId }: { taskId: string }) {
  const { data, loading, refetch } = useQuery(GET_COMMENTS, {
    variables: { taskId },
  });
  const [addComment] = useMutation(ADD_COMMENT);
  const [body, setBody] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!body.trim()) return;
    await addComment({ variables: { taskId, body: body.trim() } });
    setBody("");
    refetch();
  };

  const comments = (data as { taskComments?: { id: string; authorId: string; body: string; createdAt: string }[] } | undefined)?.taskComments ?? [];

  return (
    <div>
      <h3 className="text-lg font-semibold">Comments</h3>
      <div className="mt-4 space-y-3">
        {loading && <p className="text-sm text-muted-foreground">Loading...</p>}
        {comments.map(
          (c: { id: string; authorId: string; body: string; createdAt: string }) => (
            <div
              key={c.id}
              className="rounded-lg border border-foreground/10 bg-card p-3"
            >
              <p className="text-sm">{c.body}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {new Date(c.createdAt).toLocaleString()}
              </p>
            </div>
          )
        )}
      </div>
      <form onSubmit={handleSubmit} className="mt-4 flex gap-2">
        <Input
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Add a comment..."
          className="flex-1"
        />
        <Button type="submit" disabled={!body.trim()}>
          Post
        </Button>
      </form>
    </div>
  );
}
