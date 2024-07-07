"""Dump all tiles from an image via titiler"""

import asyncio
import os
from datetime import datetime
from functools import wraps
from urllib.parse import urljoin

import aiofiles
import aiocsv
import aiohttp
import click
import mercantile

TITILER_BASE_URL = os.getenv("TITILER_BASE_URL", "http://localhost:4000")
TITILER_INFO_ENDPOINT = urljoin(TITILER_BASE_URL, "/cog/info")
TITILER_TILE_ENDPOINT = urljoin(TITILER_BASE_URL, "/cog/tiles/{z}/{x}/{y}")


def _create_aiohttp_tracer(writer: aiocsv.AsyncWriter) -> aiohttp.TraceConfig:
    """Create aiohttp session that writes out metadata for each request to a CSV file."""

    async def _on_request_start(session, trace_config_ctx, params):
        # Using event loop for measuring request duration because it's more accurate than `datetime` here
        trace_config_ctx.start_time = asyncio.get_event_loop().time()
        trace_config_ctx.request_start_time = datetime.now().strftime("%Y-%m-%dT%H:%M:%S.%fZ")

    async def _on_request_end(session, trace_config_ctx, params):
        duration_seconds = asyncio.get_event_loop().time() - trace_config_ctx.start_time
        http_path = params.url.path
        image_url = params.url.query["url"]

        # Write out to csv
        await writer.writerow(
            [
                http_path,
                duration_seconds,
                trace_config_ctx.request_start_time,
                image_url,
            ]
        )

    trace_config = aiohttp.TraceConfig()
    trace_config.on_request_start.append(_on_request_start)
    trace_config.on_request_end.append(_on_request_end)
    return trace_config


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


async def _fetch_tile_semaphore(
    session: aiohttp.ClientSession,
    tile: mercantile.Tile,
    infile: str,
    semaphore: asyncio.Semaphore,
):
    async with semaphore:
        await _fetch_tile(session, tile, infile)


async def dump_tiles(
    session: aiohttp.ClientSession,
    infile: str,
    minzoom: int = None,
    maxzoom: int = None,
    limit: int = None,
    concurrency: int = 50,
):
    """Dump all tiles from the infil between min and max zoom."""
    cog_info = await _fetch_info(session, infile)

    minzoom = minzoom or cog_info["minzoom"]
    maxzoom = maxzoom or cog_info["maxzoom"]
    tiles = mercantile.tiles(*cog_info["bounds"], range(minzoom, maxzoom + 1))
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
@click.argument("outfile")
@click.option("--minzoom", type=int, default=None)
@click.option("--maxzoom", type=int, default=None)
@click.option("--limit", type=int, default=None)
@click.option("--concurrency", type=int, default=50)
@coro
async def main(
    infile: str,
    outfile: str,
    minzoom: int = None,
    maxzoom: int = None,
    limit: int = None,
    concurrency: int = 50,
):
    assert outfile.endswith(".csv")

    async with aiofiles.open(outfile, "w") as outf:
        writer = aiocsv.AsyncWriter(outf)
        await writer.writerow(["http_path", "duration_seconds", "start_time", "url"])

        trace_config = _create_aiohttp_tracer(writer)
        async with aiohttp.ClientSession(trace_configs=[trace_config]) as session:
            await dump_tiles(session, infile, minzoom, maxzoom, limit, concurrency)


if __name__ == "__main__":
    main()
