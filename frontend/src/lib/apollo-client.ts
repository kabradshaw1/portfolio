import { ApolloClient, InMemoryCache, HttpLink, from, Observable } from "@apollo/client";
import { setContext } from "@apollo/client/link/context";
import { ErrorLink } from "@apollo/client/link/error";
import { ServerError } from "@apollo/client/errors";
import { getAccessToken, refreshAccessToken, clearTokens, GATEWAY_URL } from "./auth";

const httpLink = new HttpLink({
  uri: `${GATEWAY_URL}/graphql`,
});

const authLink = setContext((_, { headers }) => {
  const token = getAccessToken();
  return {
    headers: {
      ...headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  };
});

const errorLink = new ErrorLink(({ error, operation, forward }) => {
  if (
    error instanceof ServerError &&
    (error.statusCode === 401 || error.statusCode === 403)
  ) {
    return new Observable((observer) => {
      refreshAccessToken()
        .then((newToken) => {
          if (!newToken) {
            clearTokens();
            window.location.href = "/java/tasks";
            observer.error(error);
            return;
          }
          const oldHeaders = operation.getContext().headers;
          operation.setContext({
            headers: {
              ...oldHeaders,
              Authorization: `Bearer ${newToken}`,
            },
          });
          forward(operation).subscribe(observer);
        })
        .catch((err) => {
          observer.error(err);
        });
    });
  }
});

export const apolloClient = new ApolloClient({
  link: from([errorLink, authLink, httpLink]),
  cache: new InMemoryCache(),
});
