export default function RootLoading() {
	return (
		<div className="min-h-screen mesh flex items-center justify-center px-6">
			<div className="flex flex-col items-center gap-4 animate-fade-in">
				<div className="w-12 h-12 rounded-2xl bg-rose-100 skeleton" />
				<div className="skeleton skeleton-block !w-48" />
				<div className="skeleton skeleton-block !w-64 !h-4" />
			</div>
		</div>
	);
}
