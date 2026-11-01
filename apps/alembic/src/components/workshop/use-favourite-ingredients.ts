import { useEffect, useState } from "react";

const STORAGE_KEY = "alembic:favourite-ingredients";

export function useFavouriteIngredients() {
	const [favourites, setFavourites] = useState<string[]>([]);

	useEffect(() => {
		try {
			const stored: unknown = JSON.parse(
				localStorage.getItem(STORAGE_KEY) ?? "[]",
			);
			if (!Array.isArray(stored)) return;
			const values: unknown[] = stored;
			const ids = values.filter(
				(value): value is string => typeof value === "string",
			);
			if (ids.length === values.length) setFavourites([...new Set(ids)]);
		} catch {
			return;
		}
	}, []);

	function toggleFavourite(id: string) {
		const next = favourites.includes(id)
			? favourites.filter((favourite) => favourite !== id)
			: [...favourites, id];
		setFavourites(next);
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
		} catch {
			return;
		}
	}

	return { favourites, toggleFavourite };
}
