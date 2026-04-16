import { ApolloClient, InMemoryCache, HttpLink, from, Observable } from "@apollo/client";
import { ErrorLink } from "@apollo/client/link/error";
import { ServerError } from "@apollo/client/errors";
import { refreshAccessToken, GATEWAY_URL } from "./auth";

const httpLink = new HttpLink({
  uri: `${GATEWAY_URL}/graphql`,
  credentials: "include",
});

const errorLink = new ErrorLink(({ error, operation, forward }) => {
  if (
    error instanceof ServerError &&
    (error.statusCode === 401 || error.statusCode === 403)
  ) {
    return new Observable((observer) => {
      refreshAccessToken()
        .then((success) => {
          if (!success) {
            window.location.href = "/java/tasks";
            observer.error(error);
            return;
          }
          forward(operation).subscribe(observer);
        })
        .catch((err) => observer.error(err));
    });
  }
});

export const apolloClient = new ApolloClient({
  link: from([errorLink, httpLink]),
  cache: new InMemoryCache(),
});
