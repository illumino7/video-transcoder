// Header presents the global branding and primary navigation context for the application.
// It accepts a toggleable subtitle to cleanly adapt between the upload and player views.
interface HeaderProps {
  showSubtitle?: boolean;
}

export default function Header({ showSubtitle = true }: HeaderProps) {
  return (
    <header className="text-center mb-12 animate-fade-in-down select-none">
      <div className="inline-flex items-center gap-2 mb-2 select-none">
        <span className="text-xs font-mono font-medium tracking-widest text-zinc-500 uppercase">
          Adaaptiv
        </span>
        <span className="h-3 w-[1px] bg-zinc-800" aria-hidden="true" />
        <span className="text-[9px] font-mono font-medium bg-zinc-900 text-zinc-500 px-1 py-0.5 rounded border border-zinc-800/80 leading-none">
          v1.0.0
        </span>
      </div>
      <h1 className="text-3xl font-semibold tracking-tight text-white mb-2 text-wrap-balance">
        Video Transcoder
      </h1>
      {showSubtitle && (
        <p className="text-zinc-500 text-sm max-w-md mx-auto leading-relaxed text-wrap-balance">
          Upload and transcode videos into adaptive HLS&nbsp;streams.
        </p>
      )}
    </header>
  );
}
