# SemCom

Semantic Compiler (SemCom) memory system / externalized parametric data for AI agents.

## Installation

For agent-guided installation, please refer to the [semcom-install](https://github.com/ars/semcom-install) repository.

How it works:
Input goes through a new type of embedding process, converting known / learned words into integers. Those integers are then processed as an array against a hierarchical cluster of ENGLISH semantic concepts, from coarsest to finest (don't ask, this is my secret sauce that took 2 years to develop.) A "semkey" is created for the input, which is an array of L0_ids which defines the semantic boundaries of the input.
Retrieval checks for memories already retrieved for the current turn's session, and filters results, then retrieves the top candidates that overlap with the input's boundaries.
Input is stored with semkey for distillation on session change.

On session change, distillation triggers, extracting information about every topic of conversation, relationships, etc and detecting new vocabulary (names, places, people, events, etc). New vocabulary is added to personal_tokens, and a wiki-style memory is created for the new personal_token (this is actively refactored whenever distillation detects the personal token again). Each distilled extraction generates a semkey, and is stored for retrieval.

Why?
1.) performance: sub ms retrieval. Faster than human perception. Scales exponentially better than vector embedding searches as well.
2.) token efficiency: the distillation process uses a cheap model (gemini-3.1-flash-lite-preview is what I use) to save costs for the model you actually want to use for responses. Think of it like MTP for RAG.
3.) reliability: traditional agent memory systems in openclaw/hermes etc rely on the model to actually record memories. This is the dumbest thing I've ever heard. LLMs are stupid when it comes to this type of function. Programmatic is infinitely more consistent.
4.) less tool calls: automatic retrieval means you waste less time / tokens waiting for the agent to find relevant information in it's messy AF memory files.
5.) externalized parametric data: This is the end goal. I want to expand this system to provide smaller models with data that will allow them to respond with quality similar to, or better than trillion parameter models.

About me:
I'm Ars. I'm not a programmer, so yes the repo is AI generated code, and is subject to slop. I do my best to guide the model to keep the repo maintainable, and use additional tools (openSynapse, reBough, etc) for my readability and verification.
My specialty lies in the vector architecture that is compiled into the system. Nothing like it exists. That's a bit of a lie really, since it's modeled after how LLMs think, and how attention mechanisms work, but in a much more efficient way. Something that I hope to explore implementing into models directly when I can afford model development.

Contributions are welcome. If you want to branch this design for your own use, I'd be happy to help. I'm currently working on developing a V2 of the core vector architecture, as well as full wikipedia "foundational knowledge" compiled databases.

Updates are likely to happen a few times a week. I might make changes that will break your database. I will post a "CHANGELOG.md" and I will note **BREAKING** changes where applicable, as well as how to preserve your data if I cannot make migration reliable programmatically.

Requirements: You will need an API key or a local model for the distillation process. The system does not function without the distillation process. Use a cheap model. Distillation is a classification and sorting task, pattern recognition, something that LLMs are inherently good at. You don't need reasoning. You do need a context window that can fit your session though, which is why my recommendation stands with Gemini-3.1-flash-lite-preview (or latest flash lite model). Deepseek v4 flash is good too, Claude Haiku, GPT Mini, and that tier of model is ideal.

That's all I feel like writing today. Look for benchmarks against popular memory systems in the coming months. I'm also planning on doing benchmarks like AMB, MemoryAgentBench, and MemoryArena soon.
