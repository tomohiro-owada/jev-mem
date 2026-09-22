"""JSON-lines bridge between jev-mem and laya-mlx.

The Go process owns this worker and keeps it alive for the duration of one
jev-mem command.  Keeping model loading on the Python side avoids a local HTTP
port while still reusing the loaded MLX weights across fingerprint batches.
"""

import argparse
import json
import sys

import laya_mlx as laya


def main():
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--model", required=True)
    parser.add_argument("--device", default="gpu")
    parser.add_argument("--revision")
    parser.add_argument("--dtype", default="float16")
    parser.add_argument("--batch-size", type=int, default=16)
    args = parser.parse_args()

    agent = laya.load(
        args.model,
        device=args.device,
        dtype=args.dtype,
        batch_size=args.batch_size,
        revision=args.revision,
    )
    print(json.dumps({"ready": True, "model": args.model}), flush=True)

    for line in sys.stdin:
        try:
            request = json.loads(line)
            # laya-mlx serializes non-string instructions with json.dumps(),
            # whose default ASCII escaping hides Japanese text from the model.
            # Pre-serialize structured jev-mem instructions as real Unicode.
            questions = request["questions"]
            for definition in questions.values():
                if not isinstance(definition.get("instructions"), str):
                    definition["instructions"] = json.dumps(
                        definition["instructions"], ensure_ascii=False
                    )
            result = agent.system_one(request["state"], questions)
            revision = f"@{args.revision}" if args.revision else ""
            result["model"] = f"laya-mlx:{args.model}{revision}"
            print(json.dumps({"ok": True, "response": result}), flush=True)
        except Exception as exc:  # return a structured error without killing the worker
            print(
                json.dumps(
                    {"ok": False, "error": f"{type(exc).__name__}: {exc}"}
                ),
                flush=True,
            )


if __name__ == "__main__":
    main()
