database: memory_db user: ars location: localhost:5432 (inside of a docker container)
public.l0
    l0_id integer NOT NULL,
    tokens integer[],
    is_noise boolean,
    membership_strength double precision,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT l0_pkey PRIMARY KEY (l0_id)

public.l1
    l1_id integer NOT NULL,
    l0_members integer[],
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT l1_pkey PRIMARY KEY (l1_id)

public.l2
    l2_id integer NOT NULL,
    l1_members integer[],
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT l2_pkey PRIMARY KEY (l2_id)

public.l3
    l3_id integer NOT NULL,
    l2_members integer[],
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT l3_pkey PRIMARY KEY (l3_id)
    
public.tokenizer
    token_id integer NOT NULL,
    word text COLLATE pg_catalog."default" NOT NULL,
    CONSTRAINT tokenizer_pkey PRIMARY KEY (token_id),
    CONSTRAINT tokenizer_word_key UNIQUE (word)
