import { useMutation } from "@tanstack/react-query";
import { KeyRound, Loader2 } from "lucide-react";
import { type FC, type FormEvent, useState } from "react";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import { Input } from "../components/ui/input";
import { AESTHETIC_CARD, AESTHETIC_CARD_HEADER } from "../lib/aesthetic";
import { login } from "../lib/api";
import { cn } from "../lib/utils";

export interface LoginCardProps {
  appName: string;
  appIcon?: string;
  /** Called once the server has accepted the secret and set the cookie. */
  onSuccess: () => void;
}

/**
 * Secret entry form shown by `AuthGate` when the API requires a credential.
 *
 * The secret is posted to `/api/auth/login` and exchanged for an HttpOnly
 * cookie, so it is never persisted anywhere JavaScript can read it — it lives
 * in component state for the duration of the submit and nowhere else.
 */
export const LoginCard: FC<LoginCardProps> = ({
  appName,
  appIcon,
  onSuccess,
}) => {
  const [secret, setSecret] = useState("");

  const loginMutation = useMutation({
    mutationFn: () => login(secret),
    onSuccess: () => {
      setSecret("");
      onSuccess();
    },
  });

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!secret || loginMutation.isPending) return;
    loginMutation.mutate();
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-app text-foreground p-4">
      <Card className={cn(AESTHETIC_CARD, "w-full max-w-sm")}>
        <CardHeader className={AESTHETIC_CARD_HEADER}>
          <CardTitle className="flex items-center gap-2 text-base">
            {appIcon ? (
              <img src={appIcon} alt="" className="size-6" />
            ) : (
              <KeyRound className="size-4" />
            )}
            Sign in to {appName}
          </CardTitle>
        </CardHeader>
        <CardContent className="pt-4">
          <form onSubmit={handleSubmit} className="flex flex-col gap-3">
            <label
              htmlFor="rotom-secret"
              className="text-sm text-muted-foreground"
            >
              API secret
            </label>
            <Input
              id="rotom-secret"
              type="password"
              value={secret}
              autoFocus
              autoComplete="current-password"
              placeholder="Enter the configured api secret"
              onChange={(event) => setSecret(event.target.value)}
              disabled={loginMutation.isPending}
            />

            {loginMutation.isError ? (
              <p className="text-sm text-destructive" role="alert">
                {loginMutation.error instanceof Error
                  ? loginMutation.error.message
                  : "Sign in failed"}
              </p>
            ) : null}

            <Button
              type="submit"
              disabled={!secret || loginMutation.isPending}
              className="mt-1"
            >
              {loginMutation.isPending ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Signing in…
                </>
              ) : (
                "Sign in"
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
};
