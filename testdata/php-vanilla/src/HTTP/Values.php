<?php

declare(strict_types=1);

namespace HTTP;

// Values is net/url's Values, which is what $r->url->query() answers: a name
// maps to a list, get takes the first, and a name that is not there is "".
final class Values
{
    /** @param array<string, mixed> $values */
    public function __construct(private array $values)
    {
    }

    public function get(string $name): string
    {
        $value = $this->values[$name] ?? '';
        if (is_array($value)) {
            $value = $value[0] ?? '';
        }

        return is_scalar($value) ? (string) $value : '';
    }

    public function has(string $name): bool
    {
        return isset($this->values[$name]);
    }

    /** @return array<string, mixed> */
    public function all(): array
    {
        return $this->values;
    }
}
