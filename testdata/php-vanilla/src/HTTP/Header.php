<?php

declare(strict_types=1);

namespace HTTP;

// Header is net/http's Header: names map to lists of values, set replaces and
// add appends, and a name is matched however it was spelled. It collects what a
// handler sets and ResponseWriter is what puts it on the response.
final class Header
{
    /** @var array<string, list<string>> */
    private array $values = [];

    public function set(string $name, string $value): void
    {
        $this->values[$this->key($name)] = [$value];
    }

    public function add(string $name, string $value): void
    {
        $this->values[$this->key($name)][] = $value;
    }

    // get answers the first value under $name, or "" for a name that is not
    // there, which is what net/http's Header.Get answers.
    public function get(string $name): string
    {
        return $this->values[$this->key($name)][0] ?? '';
    }

    public function del(string $name): void
    {
        unset($this->values[$this->key($name)]);
    }

    /** @return array<string, list<string>> */
    public function all(): array
    {
        return $this->values;
    }

    // key canonicalises a name the way net/http does, so a handler setting
    // "content-type" and one setting "Content-Type" reach the same entry.
    private function key(string $name): string
    {
        return ucwords(strtolower($name), '-');
    }
}
