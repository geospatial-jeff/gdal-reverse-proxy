"""Dump all tiles from an image via titiler"""
import asyncio
import os
from functools import wraps
from urllib.parse import urljoin

import aiohttp
import click
import mercantile

TITILER_BASE_URL = os.getenv("TITILER_BASE_URL", "http://localhost:4000")
TITILER_INFO_ENDPOINT = urljoin(TITILER_BASE_URL, "/cog/info")
TITILER_TILE_ENDPOINT = urljoin(TITILER_BASE_URL, "/cog/tiles/{z}/{x}/{y}")


async def _fetch_info(session: aiohttp.ClientSession, infile: str):
    params = {"url": infile}
    async with session.get(TITILER_INFO_ENDPOINT, params=params) as resp:
        resp.raise_for_status()
        return await resp.json()


async def _fetch_tile(session: aiohttp.ClientSession, tile: mercantile.Tile, infile: str):
    tile_url = TITILER_TILE_ENDPOINT.format(x=tile.x, y=tile.y, z=tile.z)
    params = {"url": infile}
    async with session.get(tile_url, params=params) as resp:
        if resp.status == 404:
            return
        resp.raise_for_status()
        return await resp.read()


async def _fetch_tile_semaphore(session: aiohttp.ClientSession, tile: mercantile.Tile, infile: str, semaphore: asyncio.Semaphore):
    async with semaphore:
        await _fetch_tile(session, tile, infile)


async def dump_tiles(infile: str, minzoom: int = None, maxzoom: int = None, limit: int = None, concurrency: int = 50):
    """Dump all tiles from the infil between min and max zoom."""
    async with aiohttp.ClientSession() as session:
        cog_info = await _fetch_info(session, infile)

        minzoom = minzoom or cog_info['minzoom']
        maxzoom = maxzoom or cog_info['maxzoom']
        tiles = mercantile.tiles(*cog_info['bounds'], range(minzoom, maxzoom + 1))
        if limit:
            tiles = list(tiles)[:limit]

        semaphore = asyncio.Semaphore(concurrency)
        await asyncio.gather(
            *[_fetch_tile_semaphore(session, tile, infile, semaphore) for tile in tiles]
        )  


def coro(f):
    @wraps(f)
    def wrapper(*args, **kwargs):
        return asyncio.run(f(*args, **kwargs))
    return wrapper


@click.command()
@click.argument("infile")
@click.option("--minzoom", type=int, default=None)
@click.option("--maxzoom", type=int, default=None)
@click.option("--limit", type=int, default=None)
@click.option("--concurrency", type=int, default=50)
@coro
async def main(infile: str, minzoom: int = None, maxzoom: int = None, limit: int = None, concurrency: int = 50):
    await dump_tiles(infile, minzoom, maxzoom, limit, concurrency)


if __name__ == "__main__":
    main()
