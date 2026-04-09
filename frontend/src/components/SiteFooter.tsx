export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-foreground/10 bg-background">
      <div className="mx-auto flex max-w-5xl flex-col items-center gap-2 px-6 py-6 text-sm text-muted-foreground sm:flex-row sm:justify-between">
        <p>© {new Date().getFullYear()} Kyle Bradshaw</p>
        <nav className="flex items-center gap-5">
          <a
            href="mailto:kylebradshaw.dev@gmail.com"
            className="hover:text-foreground transition-colors"
          >
            Email
          </a>
          <a
            href="https://github.com/kabradshaw1"
            target="_blank"
            rel="noopener noreferrer"
            className="hover:text-foreground transition-colors"
          >
            GitHub
          </a>
          <a
            href="https://www.linkedin.com/in/kyle-bradshaw-15950988/"
            target="_blank"
            rel="noopener noreferrer"
            className="hover:text-foreground transition-colors"
          >
            LinkedIn
          </a>
        </nav>
      </div>
    </footer>
  );
}
