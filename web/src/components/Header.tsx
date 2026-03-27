interface HeaderProps {
  showSubtitle?: boolean;
}

export default function Header({ showSubtitle = true }: HeaderProps) {
  return (
    <header className="text-center mb-12 animate-fade-in-down">
      <h1 className="text-3xl font-bold tracking-tight bg-gradient-to-br from-accent via-accent-purple to-accent-light bg-clip-text text-transparent mb-2">
        Adaaptiv - Video Transcoder 
      </h1>
      {showSubtitle && (
        <p className="text-zinc-400 text-[0.95rem]">
          Upload a video and stream it in adaptive HLS quality
        </p>
      )}
    </header>
  );
}
