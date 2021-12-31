package main

import (
	"encoding/json"
	"log"

	badger "github.com/dgraph-io/badger/v3"
)

type Repository struct {
	Db *badger.DB
}

type Data struct {
	PublicKey  string
	User       string
	Collection string
}

func (r *Repository) GetAll() (out map[string]Data, err error) {
	out = map[string]Data{}
	if err := r.Db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			mint := string(item.Key())
			if err := item.Value(func(val []byte) error {
				var data Data
				err = json.Unmarshal(val, &data)
				if err != nil {
					log.Printf("failed to unmarshall data: %v", err)

					return err
				}
				out[mint] = data
				return nil
			}); err != nil {
				log.Printf("failed to retrieve data: %v", err)

				return err
			}
		}

		return nil
	}); err != nil {
		log.Printf("failed to retrieve data for all mints: %v", err)
	}

	return out, err
}

func (r *Repository) Get(mint string) (data *Data, err error) {
	if err = r.Db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(mint))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			log.Printf("failed to retrieve data: %v", err)

			return err
		}

		return item.Value(func(val []byte) error {
			err = json.Unmarshal(val, &data)
			if err != nil {
				log.Printf("failed to unmarshall data: %v", err)
			}

			return err
		})
	}); err != nil {
		log.Printf("failed to retrieve owner: %v", err)
	}

	return data, err
}

func (r *Repository) Set(mint, data, user string) (err error) {
	if err = r.Db.Update(func(txn *badger.Txn) error {
		o := Data{PublicKey: data, User: user}
		val, err := json.Marshal(o)
		if err != nil {
			log.Printf("failed to marshall data to json: %v", err)

			return err
		}
		e := badger.NewEntry([]byte(mint), val)
		return txn.SetEntry(e)
	}); err != nil {
		log.Printf("failed to save data: %v", err)
	}

	return err
}

func (r *Repository) Delete(mint string) (err error) {
	if err = r.Db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(mint))
	}); err != nil {
		log.Printf("failed to delete data: %v", err)
	}

	return err
}
